package vfs

// Help contains text describing file and directory caching to add to
// the command help.
var Help = `
### Directory Cache ###

Using the ` + "`--dir-cache-time`" + ` flag, you can set how long a
directory should be considered up to date and not refreshed from the
backend. Changes made locally in the mount may appear immediately or
invalidate the cache. However, changes done on the remote will only
be picked up once the cache expires.

Alternatively, you can send a ` + "`SIGHUP`" + ` signal to rclone for
it to flush all directory caches, regardless of how old they are.
Assuming only one rclone instance is running, you can reset the cache
like this:

    kill -SIGHUP $(pidof rclone)

### File Caching ###

**NB** File caching is **EXPERIMENTAL** - use with care!

These flags control the file caching options.

    --cache-dir string               Directory rclone will use for caching.
    --cache-max-age duration         Max age of objects in the cache. (default 1h0m0s)
    --cache-mode string              Cache mode off|minimal|writes|full (default "off")
    --cache-poll-interval duration   Interval to poll the cache for stale objects. (default 1m0s)

If run with ` + "`-vv`" + ` rclone will print the location of the file cache.  The
files are stored in the user cache file area which is OS dependent but
can be controlled with ` + "`--cache-dir`" + ` or setting the appropriate
environment variable.

The cache has 4 different modes selected by ` + "`--cache-mode`" + `.
The higher the cache mode the more compatible rclone becomes at the
cost of using disk space.

Note that files are written back to the remote only when they are
closed so if rclone is quit or dies with open files then these won't
get written back to the remote.  However they will still be in the on
disk cache.

#### --cache-mode off ####

In this mode the cache will read directly from the remote and write
directly to the remote without caching anything on disk.

This will mean some operations are not possible

  * Files can't be opened for both read AND write
  * Files opened for write can't be seeked
  * Files open for read/write with O_TRUNC will be opened write only
  * Files open for write only will behave as if O_TRUNC was supplied
  * Open modes O_APPEND, O_TRUNC are ignored

#### --cache-mode minimal ####

This is very similar to "off" except that files opened for read AND
write will be buffered to disks.  This means that files opened for
write will be a lot more compatible, but uses the minimal disk space.

These operations are not possible

  * Files opened for write only can't be seeked
  * Files open for write only will behave as if O_TRUNC was supplied
  * Files opened for write only will ignore O_APPEND, O_TRUNC

#### --cache-mode writes ####

In this mode files opened for read only are still read directly from
the remote, write only and read/write files are buffered to disk
first.

This mode should support all normal file system operations.

#### --cache-mode full ####

In this mode all reads and writes are buffered to and from disk.  When
a file is opened for read it will be downloaded in its entirety first.

In this mode, unlike the others, when a file is written to the disk,
it will be kept on the disk after it is written to the remote.  It
will be purged on a schedule according to ` + "`--cache-max-age`" + `.

This mode should support all normal file system operations.
`
