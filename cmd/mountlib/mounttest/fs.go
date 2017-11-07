// Test suite for rclonefs

package mounttest

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ncw/rclone/fs"
	_ "github.com/ncw/rclone/fs/all" // import all the file systems
	"github.com/ncw/rclone/fstest"
	"github.com/ncw/rclone/vfs"
	"github.com/ncw/rclone/vfs/vfsflags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	// UnmountFn is called to unmount the file system
	UnmountFn func() error
	// MountFn is called to mount the file system
	MountFn func(f fs.Fs, mountpoint string) (*vfs.VFS, <-chan error, func() error, error)
)

var (
	mountFn MountFn
)

// TestMain drives the tests
func TestMain(m *testing.M, fn MountFn) {
	mountFn = fn
	flag.Parse()
	var rc int
	cacheModes := []vfs.CacheMode{
		vfs.CacheModeOff,
		vfs.CacheModeMinimal,
		vfs.CacheModeWrites,
		vfs.CacheModeFull,
	}
	for _, cacheMode := range cacheModes {
		vfsflags.Opt.CacheMode = cacheMode
		log.Printf("Starting test run with cache mode %v", cacheMode)
		run = newRun()
		rc = m.Run()
		run.Finalise()
		log.Printf("Finished test run with cache mode %v", cacheMode)
		if rc != 0 {
			break
		}
	}
	vfsflags.Opt.CacheMode = vfs.DefaultOpt.CacheMode
	os.Exit(rc)
}

// Run holds the remotes for a test run
type Run struct {
	vfs          *vfs.VFS
	mountPath    string
	fremote      fs.Fs
	fremoteName  string
	cleanRemote  func()
	umountResult <-chan error
	umountFn     UnmountFn
	skip         bool
}

// run holds the master Run data
var run *Run

// newRun initialise the remote mount for testing and returns a run
// object.
//
// r.fremote is an empty remote Fs
//
// Finalise() will tidy them away when done.
func newRun() *Run {
	r := &Run{
		umountResult: make(chan error, 1),
	}

	fstest.Initialise()

	var err error
	r.fremote, r.fremoteName, r.cleanRemote, err = fstest.RandomRemote(*fstest.RemoteName, *fstest.SubDir)
	if err != nil {
		log.Fatalf("Failed to open remote %q: %v", *fstest.RemoteName, err)
	}

	err = r.fremote.Mkdir("")
	if err != nil {
		log.Fatalf("Failed to open mkdir %q: %v", *fstest.RemoteName, err)
	}

	if runtime.GOOS != "windows" {
		r.mountPath, err = ioutil.TempDir("", "rclonefs-mount")
		if err != nil {
			log.Fatalf("Failed to create mount dir: %v", err)
		}
	} else {
		// Find a free drive letter
		drive := ""
		for letter := 'E'; letter <= 'Z'; letter++ {
			drive = string(letter) + ":"
			_, err := os.Stat(drive + "\\")
			if os.IsNotExist(err) {
				goto found
			}
		}
		log.Fatalf("Couldn't find free drive letter for test")
	found:
		r.mountPath = drive
	}

	// Mount it up
	r.mount()

	return r
}

func (r *Run) mount() {
	log.Printf("mount %q %q", r.fremote, r.mountPath)
	var err error
	r.vfs, r.umountResult, r.umountFn, err = mountFn(r.fremote, r.mountPath)
	if err != nil {
		log.Printf("mount failed: %v", err)
		r.skip = true
	}
	log.Printf("mount OK")
}

func (r *Run) umount() {
	if r.skip {
		log.Printf("FUSE not found so skipping umount")
		return
	}
	/*
		log.Printf("Calling fusermount -u %q", r.mountPath)
		err := exec.Command("fusermount", "-u", r.mountPath).Run()
		if err != nil {
			log.Printf("fusermount failed: %v", err)
		}
	*/
	log.Printf("Unmounting %q", r.mountPath)
	err := r.umountFn()
	if err != nil {
		log.Printf("signal to umount failed - retrying: %v", err)
		time.Sleep(3 * time.Second)
		err = r.umountFn()
	}
	if err != nil {
		log.Fatalf("signal to umount failed: %v", err)
	}
	log.Printf("Waiting for umount")
	err = <-r.umountResult
	if err != nil {
		log.Fatalf("umount failed: %v", err)
	}

	// Cleanup the VFS cache - umount has called Shutdown
	err = r.vfs.CleanUp()
	if err != nil {
		log.Printf("Failed to cleanup the VFS cache: %v", err)
	}
}

func (r *Run) skipIfNoFUSE(t *testing.T) {
	if r.skip {
		t.Skip("FUSE not found so skipping test")
	}
}

// Finalise cleans the remote and unmounts
func (r *Run) Finalise() {
	r.umount()
	r.cleanRemote()
	err := os.RemoveAll(r.mountPath)
	if err != nil {
		log.Printf("Failed to clean mountPath %q: %v", r.mountPath, err)
	}
}

func (r *Run) path(filepath string) string {
	// return windows drive letter root as E:/
	if filepath == "" && runtime.GOOS == "windows" {
		return run.mountPath + "/"
	}
	return path.Join(run.mountPath, filepath)
}

type dirMap map[string]struct{}

// Create a dirMap from a string
func newDirMap(dirString string) (dm dirMap) {
	dm = make(dirMap)
	for _, entry := range strings.Split(dirString, "|") {
		if entry != "" {
			dm[entry] = struct{}{}
		}
	}
	return dm
}

// Returns a dirmap with only the files in
func (dm dirMap) filesOnly() dirMap {
	newDm := make(dirMap)
	for name := range dm {
		if !strings.HasSuffix(name, "/") {
			newDm[name] = struct{}{}
		}
	}
	return newDm
}

// reads the local tree into dir
func (r *Run) readLocal(t *testing.T, dir dirMap, filepath string) {
	realPath := r.path(filepath)
	files, err := ioutil.ReadDir(realPath)
	require.NoError(t, err)
	for _, fi := range files {
		name := path.Join(filepath, fi.Name())
		if fi.IsDir() {
			dir[name+"/"] = struct{}{}
			r.readLocal(t, dir, name)
			assert.Equal(t, run.vfs.Opt.DirPerms&os.ModePerm, fi.Mode().Perm())
		} else {
			dir[fmt.Sprintf("%s %d", name, fi.Size())] = struct{}{}
			assert.Equal(t, run.vfs.Opt.FilePerms&os.ModePerm, fi.Mode().Perm())
		}
	}
}

// reads the remote tree into dir
func (r *Run) readRemote(t *testing.T, dir dirMap, filepath string) {
	objs, dirs, err := fs.WalkGetAll(r.fremote, filepath, true, 1)
	if err == fs.ErrorDirNotFound {
		return
	}
	require.NoError(t, err)
	for _, obj := range objs {
		dir[fmt.Sprintf("%s %d", obj.Remote(), obj.Size())] = struct{}{}
	}
	for _, d := range dirs {
		name := d.Remote()
		dir[name+"/"] = struct{}{}
		r.readRemote(t, dir, name)
	}
}

// checkDir checks the local and remote against the string passed in
func (r *Run) checkDir(t *testing.T, dirString string) {
	dm := newDirMap(dirString)
	localDm := make(dirMap)
	r.readLocal(t, localDm, "")
	remoteDm := make(dirMap)
	r.readRemote(t, remoteDm, "")
	// Ignore directories for remote compare
	assert.Equal(t, dm.filesOnly(), remoteDm.filesOnly(), "expected vs remote")
	assert.Equal(t, dm, localDm, "expected vs fuse mount")
}

func (r *Run) createFile(t *testing.T, filepath string, contents string) {
	filepath = r.path(filepath)
	err := ioutil.WriteFile(filepath, []byte(contents), 0600)
	require.NoError(t, err)
}

func (r *Run) readFile(t *testing.T, filepath string) string {
	filepath = r.path(filepath)
	result, err := ioutil.ReadFile(filepath)
	require.NoError(t, err)
	return string(result)
}

func (r *Run) mkdir(t *testing.T, filepath string) {
	filepath = r.path(filepath)
	err := os.Mkdir(filepath, 0700)
	require.NoError(t, err)
}

func (r *Run) rm(t *testing.T, filepath string) {
	filepath = r.path(filepath)
	err := os.Remove(filepath)
	require.NoError(t, err)
}

func (r *Run) rmdir(t *testing.T, filepath string) {
	filepath = r.path(filepath)
	err := os.Remove(filepath)
	require.NoError(t, err)
}

// TestMount checks that the Fs is mounted by seeing if the mountpoint
// is in the mount output
func TestMount(t *testing.T) {
	run.skipIfNoFUSE(t)

	out, err := exec.Command("mount").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), run.mountPath)
}

// TestRoot checks root directory is present and correct
func TestRoot(t *testing.T) {
	run.skipIfNoFUSE(t)

	fi, err := os.Lstat(run.mountPath)
	require.NoError(t, err)
	assert.True(t, fi.IsDir())
	assert.Equal(t, run.vfs.Opt.DirPerms&os.ModePerm, fi.Mode().Perm())
}
