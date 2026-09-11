package p0

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestProcessHelper(t *testing.T) {
	role := os.Getenv("RUNWEAVE_P0_HELPER")
	if role == "" {
		return
	}
	mode := os.Getenv("RUNWEAVE_P0_MODE")
	signals := make(chan os.Signal, 1)
	if mode == "kill" {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(signals, syscall.SIGTERM)
	}
	if role == "child" {
		fmt.Printf("READY %d\n", os.Getpid())
		if mode == "kill" {
			for {
				time.Sleep(time.Hour)
			}
		}
		<-signals
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	child.Env = append(os.Environ(), "RUNWEAVE_P0_HELPER=child")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	if err := child.Wait(); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestLinuxProcessGroup(t *testing.T) {
	// Adopt orphaned grandchildren so KILL verification does not depend on WSL PID 1 reaping.
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	defer unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 0, 0, 0, 0)
	for _, mode := range []string{"term", "kill"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
			cmd.Env = append(os.Environ(), "RUNWEAVE_P0_HELPER=parent", "RUNWEAVE_P0_MODE="+mode)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			cmd.Stdout, cmd.Stderr = writer, writer
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			writer.Close()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			ready := make(chan string, 1)
			drained := make(chan error, 1)
			go func() {
				r := bufio.NewReader(reader)
				line, err := r.ReadString('\n')
				ready <- strings.TrimSpace(line)
				if err == nil {
					_, err = io.Copy(io.Discard, r)
				}
				drained <- err
			}()
			var line string
			select {
			case line = <-ready:
			case <-ctx.Done():
				t.Fatal("child readiness timed out")
			}
			fields := strings.Fields(line)
			if len(fields) != 2 || fields[0] != "READY" {
				t.Fatalf("unexpected child readiness: %q", line)
			}
			childPID, err := strconv.Atoi(fields[1])
			if err != nil {
				t.Fatal(err)
			}
			if group, err := syscall.Getpgid(childPID); err != nil || group != cmd.Process.Pid {
				t.Fatalf("child group=%d: %v", group, err)
			}
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			if mode == "kill" {
				select {
				case err := <-done:
					t.Fatalf("TERM unexpectedly stopped helper: %v", err)
				case <-time.After(150 * time.Millisecond):
				}
				if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if mode == "term" && err != nil {
					t.Fatal(err)
				}
				if mode == "kill" {
					exit, ok := err.(*exec.ExitError)
					if !ok || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
						t.Fatalf("expected KILL: %v", err)
					}
				}
			case <-ctx.Done():
				t.Fatal("parent not reaped")
			}
			if mode == "kill" {
				for {
					var status syscall.WaitStatus
					pid, err := syscall.Wait4(childPID, &status, syscall.WNOHANG, nil)
					if err != nil {
						t.Fatal(err)
					}
					if pid == childPID {
						if status.Signal() != syscall.SIGKILL {
							t.Fatalf("child status: %v", status)
						}
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("child not reaped")
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			select {
			case err := <-drained:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("output pipe not drained")
			}
			if err := syscall.Kill(childPID, 0); err != syscall.ESRCH {
				t.Fatalf("child still present: %v", err)
			}
			if err := syscall.Kill(-cmd.Process.Pid, 0); err != syscall.ESRCH {
				t.Fatalf("group still present: %v", err)
			}
		})
	}
}
