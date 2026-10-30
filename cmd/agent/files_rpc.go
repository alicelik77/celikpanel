package main

import (
	"errors"
	"os"
	"strings"
	"syscall"

	"github.com/alicelik/celikpanel/internal/transport"
)

// FileInfo represents a file or directory
type FileInfo = transport.FileInfo

// ListFilesRequest for listing directory contents
type ListFilesRequest = transport.ListFilesRequest

// ListFilesResponse contains directory listing
type ListFilesResponse = transport.ListFilesResponse

// ReadFileRequest for reading file content
type ReadFileRequest = transport.ReadFileRequest

// ReadFileResponse contains file content
type ReadFileResponse = transport.ReadFileResponse

// WriteFileRequest for writing file content
type WriteFileRequest = transport.WriteFileRequest

// CreateRequest for creating file or directory
type CreateRequest = transport.CreateFileRequest

// DeleteRequest for deleting file or directory
type DeleteRequest = transport.DeleteFileRequest

// ChmodRequest for changing permissions
type ChmodRequest = transport.ChmodFileRequest

// RenameRequest for renaming/moving files
type RenameRequest = transport.RenameFileRequest

// UploadFileRequest for uploading files
type UploadFileRequest = transport.UploadFileRequest

// File Manager RPC Methods

// siteFileAnswer turns the file system's own "not there" and "already there"
// into the exact answers the Panel classifies (transport.SiteFilePathMissing
// and friends); every other error passes unchanged. Until 30 Oct 2026 a path
// that did not exist reached the browser as a bare 500 INTERNAL.
// siteFileAnswer, dosya sisteminin "yok" ve "zaten var" yanıtlarını Panel'in
// sınıflandırdığı tam yanıtlara çevirir; diğer her hata değişmeden geçer.
func siteFileAnswer(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrNotExist) && strings.Contains(err.Error(), "site document root"):
		return errors.New(transport.SiteFileRootMissing)
	case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return errors.New(transport.SiteFilePathMissing)
	case errors.Is(err, os.ErrExist):
		return errors.New(transport.SiteFilePathExists)
	default:
		return err
	}
}

// ListFiles lists directory contents
func (a *Agent) ListFiles(req *ListFilesRequest, resp *ListFilesResponse) error {
	return siteFileAnswer(secureListSiteFiles(req, resp))
}

// ReadFile reads file content
func (a *Agent) ReadFile(req *ReadFileRequest, resp *ReadFileResponse) error {
	return siteFileAnswer(secureReadSiteFile(req, resp))
}

// WriteFile writes content to file
func (a *Agent) WriteFile(req *WriteFileRequest, resp *bool) error {
	return siteFileAnswer(secureWriteSiteFile(req, resp))
}

// CreateFileOrDir creates a file or directory
func (a *Agent) CreateFileOrDir(req *CreateRequest, resp *bool) error {
	return siteFileAnswer(secureCreateSiteFile(req, resp))
}

// DeleteFileOrDir deletes a file or directory
func (a *Agent) DeleteFileOrDir(req *DeleteRequest, resp *bool) error {
	return siteFileAnswer(secureDeleteSiteFile(req, resp))
}

// ChmodFile changes file permissions
func (a *Agent) ChmodFile(req *ChmodRequest, resp *bool) error {
	return siteFileAnswer(secureChmodSiteFile(req, resp))
}

// RenameFile renames or moves a file
func (a *Agent) RenameFile(req *RenameRequest, resp *bool) error {
	return siteFileAnswer(secureRenameSiteFile(req, resp))
}

// UploadFile handles file upload
func (a *Agent) UploadFile(req *UploadFileRequest, resp *bool) error {
	return siteFileAnswer(secureUploadSiteFile(req, resp))
}

// Helper function to parse octal permissions
func parseOctalPermissions(perm string) (os.FileMode, error) {
	if len(perm) == 4 && perm[0] == '0' {
		perm = perm[1:]
	}
	if len(perm) != 3 {
		return 0, os.ErrInvalid
	}
	var mode os.FileMode
	for _, c := range perm {
		if c < '0' || c > '7' {
			return 0, os.ErrInvalid
		}
		mode = mode*8 + os.FileMode(c-'0')
	}
	return mode, nil
}
