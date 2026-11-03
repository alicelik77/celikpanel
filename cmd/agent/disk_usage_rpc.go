package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Subscription disk usage (alpha.118, D-044): the subscription's hosting
// folder, the mail folders of its domains and its databases. Folders are
// walked by du at the lowest CPU and I/O priority (nice 19, ionice idle),
// counting allocated blocks on one file system. CelikPanel never edits fstab,
// the kernel command line or file system flags; when the owner mounted the
// hosting file system with project quotas this is reported, not used (there
// is no project id CelikPanel set to read).
//
// Abonelik disk kullanımı: barındırma klasörü, posta klasörleri ve
// veritabanları. Klasörler en düşük öncelikle du ile taranır. CelikPanel
// fstab'a, çekirdek satırına ya da dosya sistemi bayraklarına hiç dokunmaz.

const diskUsageTimeout = 9 * time.Minute

var (
	// diskUsageMu: one measurement at a time, so two walks never compete.
	diskUsageMu sync.Mutex
	// diskUsageWalk measures one folder in bytes (a seam for tests).
	diskUsageWalk = walkDiskUsage
	// diskUsageDatabases measures databases in bytes (a seam for tests).
	diskUsageDatabases = measureDatabaseBytes
	diskUsageNameRE    = regexp.MustCompile(`^[A-Za-z0-9_$-]{1,64}$`)
)

// walkDiskUsage: du -s -x -B1 at idle priority. A missing folder is 0.
func walkDiskUsage(ctx context.Context, path string) (int64, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	args := []string{"-n", "19"}
	if ionice, err := exec.LookPath("ionice"); err == nil {
		args = append(args, ionice, "-c", "3")
	}
	args = append(args, "du", "-s", "-x", "-B1", "--", path)
	output, err := exec.CommandContext(ctx, "nice", args...).Output()
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		if err == nil {
			err = errors.New("du gave no answer")
		}
		return 0, err
	}
	value, parseErr := strconv.ParseInt(fields[0], 10, 64)
	if parseErr != nil {
		return 0, fmt.Errorf("du answered %q", fields[0])
	}
	// du exits non-zero when one file could not be read (a file removed
	// during the walk); the total it printed is still the best measure.
	return value, nil
}

func quoteSQLNames(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, "'"+name+"'")
	}
	return strings.Join(quoted, ",")
}

// parseSizeMarker reads the one "size=<n>" line of a statement's answer.
func parseSizeMarker(output []byte) (int64, error) {
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "size="); ok {
			return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
	}
	return 0, errors.New("the database engine's answer could not be read")
}

// measureDatabaseBytes asks each engine for the size of the named
// databases: MariaDB/MySQL the data and index length of their tables,
// PostgreSQL pg_database_size.
func measureDatabaseBytes(databases []transport.SubscriptionDiskDatabase) (int64, []string) {
	byEngine := map[string][]string{}
	ports := map[string]int{}
	var unknown []string
	for _, database := range databases {
		if !diskUsageNameRE.MatchString(database.Name) {
			unknown = append(unknown, "database "+database.Name)
			continue
		}
		key := database.Engine
		if database.Engine == "postgresql" {
			key = "postgresql:" + strconv.Itoa(database.Port)
			ports[key] = database.Port
		}
		byEngine[key] = append(byEngine[key], database.Name)
	}
	var total int64
	for key, names := range byEngine {
		var output []byte
		var err error
		switch {
		case key == "mysql":
			output, err = mysqlExecStatement("SELECT CONCAT('size=', COALESCE(SUM(data_length + index_length), 0)) FROM information_schema.tables WHERE table_schema IN (" + quoteSQLNames(names) + ");")
		case strings.HasPrefix(key, "postgresql:"):
			var run func(string) ([]byte, error)
			run, err = postgreSQLStatementFor(ports[key])
			if err == nil {
				output, err = run("SELECT 'size=' || COALESCE(SUM(pg_database_size(datname)), 0) FROM pg_database WHERE datname IN (" + quoteSQLNames(names) + ");")
			}
		default:
			err = fmt.Errorf("unknown engine %q", key)
		}
		if err == nil {
			var size int64
			if size, err = parseSizeMarker(output); err == nil {
				total += size
				continue
			}
		}
		unknown = append(unknown, "databases ("+strings.SplitN(key, ":", 2)[0]+")")
	}
	return total, unknown
}

// SubscriptionDiskUsageV1 measures one subscription. Read only; one
// measurement at a time.
//
// SubscriptionDiskUsageV1 bir aboneliği ölçer; salt-okur, her seferinde bir.
func (a *Agent) SubscriptionDiskUsageV1(req *transport.SubscriptionDiskUsageRequest, resp *transport.SubscriptionDiskUsageResponse) error {
	if resp == nil {
		return nil
	}
	*resp = transport.SubscriptionDiskUsageResponse{Method: "walk", FreePercent: -1}
	if req == nil || req.SubscriptionID <= 0 {
		resp.Error = "a subscription is required"
		return nil
	}
	diskUsageMu.Lock()
	defer diskUsageMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), diskUsageTimeout)
	defer cancel()

	root := filepath.Join(hostingpath.SubscriptionsRoot(), strconv.Itoa(req.SubscriptionID))
	if web, err := diskUsageWalk(ctx, root); err != nil {
		resp.Unknown = append(resp.Unknown, "site files")
	} else {
		resp.WebBytes = web
	}
	for _, domain := range req.MailDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" || strings.ContainsAny(domain, "/\\") || strings.HasPrefix(domain, ".") {
			resp.Unknown = append(resp.Unknown, "mail of "+domain)
			continue
		}
		mail, err := diskUsageWalk(ctx, filepath.Join(mailRootDir, domain))
		if err != nil {
			resp.Unknown = append(resp.Unknown, "mail of "+domain)
			continue
		}
		resp.MailBytes += mail
	}
	if len(req.Databases) > 0 {
		dbBytes, unknown := diskUsageDatabases(req.Databases)
		resp.DBBytes = dbBytes
		resp.Unknown = append(resp.Unknown, unknown...)
	}
	resp.FreePercent, resp.ProjectQuota = hostingFileSystemFacts(hostingpath.SubscriptionsRoot())
	return nil
}
