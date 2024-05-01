package main

import (
	// "log"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type TempFile struct {
	fd        int
	temp      *os.File
	finalPath string
	o_tmpfile bool
}

func NewTempFile(dir, filename, finalPath string) (*TempFile, error) {
	fd, err := unix.Open(dir, unix.O_RDWR|unix.O_TMPFILE|unix.O_CLOEXEC, 0600)
	switch err {
	case nil:
		path := "/proc/self/fd/" + strconv.FormatUint(uint64(fd), 10)
		f := os.NewFile(uintptr(fd), path)
		// log.Printf("SUPPORTED")
		return &TempFile{
			fd:        fd,
			temp:      f,
			finalPath: finalPath,
			o_tmpfile: true,
		}, nil
	case syscall.EISDIR:
		// log.Printf("ISDIR")
		break // Kernel used does not support O_TMPFILE
	case syscall.EOPNOTSUPP:
		// log.Printf("OPNOTSUPP")
		break // File system (i.e. unionfs) used does not support O_TMPFILE
	default:
		// log.Printf("Creating new temp file %s failed. Error: %v\n", path, err)
		return nil, &os.PathError{
			Op:   "open",
			Path: dir,
			Err:  err,
		}
	}

	// log.Printf("Fallback")

	// Just seconds. Doesn't need to be more precise.
	timePrefixFormat := "20060102150405"
	timePrefix := time.Now().Format(timePrefixFormat) + "_"
	// Note: Go CreateTemp file prefix has also numeric form, i.e. 2759516091
	// It is just a random 32-bit integer. Go repeatedly (up to 10000 times) tries
	// new random values and tries to do OpenFile(name, O_RDWR|O_CREATE|O_EXCL, 0600) on it.
	// The O_CREATE|O_EXCL enaures only one process / thread can successfully create a file,
	// and that it cannot exist before.
	temp, err := os.CreateTemp(dir, timePrefix+filename+".*")
	if err != nil {
		return nil, err
	}
	return &TempFile{
		temp:      temp,
		finalPath: finalPath,
	}, nil
}

func (t *TempFile) File() *os.File {
	return t.temp
}

func (t *TempFile) Cleanup() error {
	if t.fd < 0 {
		// Already cleaned or moved to final destination
		return nil
	}
	if t.temp == nil {
		// Already cleaned or moved to final destination
		return nil
	}
	err1 := t.temp.Close()
	t.fd = -1
	var err2 error
	if !t.o_tmpfile {
		err2 = os.Remove(t.temp.Name())
	}
	t.temp = nil
	if err2 != nil {
		return err2
	}
	return err1
}

func (t *TempFile) Finalize() error {
	if !t.o_tmpfile {
		// If we do not use tmpfile. Call close first, then rename.
		// Otherwise rename my succeed, but close not, i.e. due to
		// flushing some buffers, disk IO error, or NFS server
		// running out of space.
		err := t.temp.Close()
		if err != nil {
			// Call to Cleanup will retry close and removal of temp file.
			return err
		}
		err = os.Rename(t.temp.Name(), t.finalPath)
		if err == nil {
			// Prevent cleanup closing and try to remove the (no non-existent) temp file.
			t.temp = nil
		}
		// If Rename failed, allow Cleanup to try to Remove tempfile at least.
		return err
	} else {
		// This requires caller to have CAP_DAC_READ_SEARCH
		// unix.linkat(fd, "", unix.AT_FDCWD, t.finalPATH, unix.AT_EMPTY_PATH);

		// When using tmpfile, rename file first instead. As we do a
		// rename use "/proc/self/fd/X", but closing it, would prevent
		// use using it in linkat.
		err := unix.Linkat(unix.AT_FDCWD, t.temp.Name(), unix.AT_FDCWD, t.finalPath,
			unix.AT_SYMLINK_FOLLOW)
		if err != nil {
			err = &os.LinkError{
				Op:  "link",
				Old: t.temp.Name(),
				New: t.finalPath,
				Err: err,
			}
		}
		err2 := t.temp.Close()
		t.temp = nil // Prevent cleanup calling Close again.
		if err != nil {
			return err
		}
		return err2
	}
}
