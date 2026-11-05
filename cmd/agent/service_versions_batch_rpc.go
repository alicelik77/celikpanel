package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// ServiceCandidateVersions is ServiceCandidateVersion for several catalogue
// services with ONE `apt-cache policy p1 … pN` (alpha.125). The Server setup
// catalogue used to ask twelve times in a row within a 6-second budget, and
// silently showed no version for whatever the budget did not reach. Each id is
// answered by exactly the rule ServiceCandidateVersion applies. A dnf host is
// asked service by service, as before.
//
// ServiceCandidateVersions, birden çok katalog servisinin kurulum adayını tek
// `apt-cache policy` çağrısıyla okur; her id ServiceCandidateVersion ile aynı
// kuralla yanıtlanır.
func (a *Agent) ServiceCandidateVersions(req *transport.ServiceCandidateVersionsRequest, reply *transport.ServiceCandidateVersionsResponse) error {
	if req == nil || reply == nil {
		return fmt.Errorf("service candidate versions request and reply are required")
	}
	if len(req.IDs) > len(core.ManagedServices) {
		return fmt.Errorf("too many services asked: %d", len(req.IDs))
	}
	*reply = transport.ServiceCandidateVersionsResponse{Versions: map[string]string{}, Errors: map[string]string{}}
	services := make([]*core.ManagedService, 0, len(req.IDs))
	for _, id := range req.IDs {
		svc := core.GetManagedServiceByID(id)
		if svc == nil {
			reply.Errors[id] = fmt.Sprintf("unknown catalog service %q", id)
			continue
		}
		services = append(services, svc)
	}
	if len(services) == 0 {
		return nil
	}
	profile, err := readHostProfile()
	if err != nil {
		return err
	}
	family := string(profile.PackageManager)
	switch family {
	case "apt":
		executable, err := executableForProfile(profile, family, "apt-cache")
		if err != nil {
			return err
		}
		candidateVersionsForServices(services, func(args ...string) ([]byte, error) {
			cmd := serviceMutationCommand(context.Background(), executable, args...)
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
			return cmd.Output()
		}, reply)
		return nil
	case "dnf":
		for _, svc := range services {
			var version string
			if err := a.ServiceCandidateVersion(&InstallServiceRequest{ID: svc.ID}, &version); err != nil {
				reply.Errors[svc.ID] = err.Error()
			} else {
				reply.Versions[svc.ID] = version
			}
		}
		return nil
	default:
		return fmt.Errorf("candidate version lookup is not supported for package-manager family %q", family)
	}
}

// candidateVersionsForServices runs one `apt-cache policy` for the first
// package of every service and gives each service the answer
// candidateVersionForService would have given it from its own call.
func candidateVersionsForServices(services []*core.ManagedService, run func(args ...string) ([]byte, error), reply *transport.ServiceCandidateVersionsResponse) {
	args := []string{"policy"}
	asked := map[string]bool{}
	for _, svc := range services {
		packages := svc.Packages["apt"]
		if len(packages) == 0 {
			reply.Errors[svc.ID] = fmt.Sprintf("service %q has no package mapping for family %q", svc.ID, "apt")
			continue
		}
		if !asked[packages[0]] {
			asked[packages[0]] = true
			args = append(args, packages[0])
		}
	}
	if len(asked) == 0 {
		return
	}
	out, err := run(args...)
	blocks := map[string]string{}
	if err == nil {
		blocks = splitAptPolicyBlocks(string(out))
	}
	for _, svc := range services {
		if _, done := reply.Errors[svc.ID]; done {
			continue
		}
		if err != nil {
			reply.Errors[svc.ID] = fmt.Sprintf("query candidate version for service %q: %v", svc.ID, err)
			continue
		}
		block, ok := blocks[svc.Packages["apt"][0]]
		if !ok {
			reply.Errors[svc.ID] = fmt.Sprintf("package index response has no Candidate field for service %q", svc.ID)
			continue
		}
		// The service's own block, read by the single-service rule.
		version, err := candidateVersionForService(svc, "apt", func(string, ...string) ([]byte, error) {
			return []byte(block), nil
		})
		if err != nil {
			reply.Errors[svc.ID] = err.Error()
			continue
		}
		reply.Versions[svc.ID] = version
	}
}

// splitAptPolicyBlocks splits `apt-cache policy p1 p2 …` into each package's
// block. A block starts with the package name and a colon at the start of a
// line ("nginx:"; a foreign architecture as "libc6:i386:"); its lines are
// indented. A package apt does not know has no block at all.
func splitAptPolicyBlocks(output string) map[string]string {
	blocks := map[string]string{}
	name := ""
	var body strings.Builder
	flush := func() {
		if name == "" {
			return
		}
		if _, seen := blocks[name]; !seen {
			blocks[name] = body.String()
		}
		if base, _, qualified := strings.Cut(name, ":"); qualified {
			if _, seen := blocks[base]; !seen {
				blocks[base] = body.String()
			}
		}
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" && line[0] != ' ' && line[0] != '\t' && strings.HasSuffix(line, ":") {
			flush()
			name = strings.TrimSuffix(line, ":")
			body.Reset()
			continue
		}
		if name != "" {
			body.WriteString(line + "\n")
		}
	}
	flush()
	return blocks
}
