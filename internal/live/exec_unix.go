//go:build unix

package live

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// maxBuffer caps what one command may print on each of stdout and stderr.
const maxBuffer = 1 << 16

// capped keeps the first maxBuffer bytes and cancels the run when a command prints more.
type capped struct {
	buf      []byte
	exceeded atomic.Bool
	cancel   context.CancelFunc
}

func (c *capped) Write(p []byte) (int, error) {
	if room := maxBuffer - len(c.buf); len(p) > room {
		c.buf = append(c.buf, p[:max(room, 0)]...)

		if c.exceeded.CompareAndSwap(false, true) {
			c.cancel()
		}

		return len(p), nil
	}

	c.buf = append(c.buf, p...)

	return len(p), nil
}

// exec runs `/bin/sh -c cmd` in the state directory with a timeout and returns the first line of its output.
func (r *Runner) exec(cmd string) result {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout())
	defer cancel()

	c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
	if st, err := os.Stat(r.Paths.Home); err == nil && st.IsDir() {
		c.Dir = r.Paths.Home
	}

	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = time.Second

	out, errOut := &capped{cancel: cancel}, &capped{cancel: cancel}
	c.Stdout, c.Stderr = out, errOut

	err := c.Run()

	switch {
	case out.exceeded.Load():
		return result{err: strp("stdout maxBuffer length exceeded")}
	case errOut.exceeded.Load():
		return result{err: strp("stderr maxBuffer length exceeded")}
	case err != nil:
		if len(errOut.buf) > 0 {
			return result{err: strp(clip(string(errOut.buf)))}
		}

		return result{err: strp(clip("Command failed: " + cmd + "\n"))}
	}

	return result{value: strp(clip(string(out.buf)))}
}
