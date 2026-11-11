// Package adb wraps the adb binary: one transport for every shell command
// Sapu runs on the phone, with a timeout on every call.
package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Device is one entry of `adb devices -l`.
type Device struct {
	Serial string
	State  string
	Model  string
}

// Client runs commands on one device.
type Client struct {
	Binary  string
	Serial  string
	Timeout time.Duration
	Debug   bool
}

// ErrNoDevice is returned when adb lists no device in the "device" state.
var ErrNoDevice = errors.New("no Android device connected (enable USB debugging and run `adb devices`)")

// FindBinary resolves adb from $SAPU_ADB, PATH or the usual SDK locations.
func FindBinary() (string, error) {
	if v := os.Getenv("SAPU_ADB"); v != "" {
		return v, nil
	}
	if p, err := exec.LookPath("adb"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		os.Getenv("ANDROID_HOME") + "/platform-tools/adb",
		os.Getenv("ANDROID_SDK_ROOT") + "/platform-tools/adb",
		home + "/Android/Sdk/platform-tools/adb",
		home + "/Library/Android/sdk/platform-tools/adb",
		home + "/AppData/Local/Android/Sdk/platform-tools/adb.exe",
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", errors.New("adb not found: install Android platform-tools (brew install android-platform-tools) or set SAPU_ADB")
}

// Devices lists connected devices.
func Devices(ctx context.Context, binary string) ([]Device, error) {
	out, err := exec.CommandContext(ctx, binary, "devices", "-l").Output()
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w", err)
	}
	var devs []Device
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		d := Device{Serial: f[0], State: f[1]}
		for _, kv := range f[2:] {
			if strings.HasPrefix(kv, "model:") {
				d.Model = strings.TrimPrefix(kv, "model:")
			}
		}
		devs = append(devs, d)
	}
	return devs, nil
}

// Connect picks the device: SAPU_SERIAL / --serial, or the single connected one.
func Connect(ctx context.Context, serial string) (*Client, error) {
	bin, err := FindBinary()
	if err != nil {
		return nil, err
	}
	devs, err := Devices(ctx, bin)
	if err != nil {
		return nil, err
	}
	var ready []Device
	for _, d := range devs {
		if d.State == "device" {
			ready = append(ready, d)
		}
	}
	if serial == "" {
		serial = os.Getenv("SAPU_SERIAL")
	}
	if serial == "" {
		switch len(ready) {
		case 0:
			if len(devs) > 0 {
				return nil, fmt.Errorf("device %s is %q: unlock the phone and accept the USB debugging prompt", devs[0].Serial, devs[0].State)
			}
			return nil, ErrNoDevice
		case 1:
			serial = ready[0].Serial
		default:
			var names []string
			for _, d := range ready {
				names = append(names, d.Serial+" ("+d.Model+")")
			}
			return nil, fmt.Errorf("%d devices connected, pick one with --serial: %s", len(ready), strings.Join(names, ", "))
		}
	}
	return &Client{Binary: bin, Serial: serial, Timeout: 20 * time.Second, Debug: os.Getenv("SAPU_DEBUG") == "1"}, nil
}

// Shell runs one shell command line on the device and returns stdout with CRLF normalized.
func (c *Client) Shell(ctx context.Context, cmd string) (string, error) {
	return c.ShellTimeout(ctx, c.Timeout, cmd)
}

// ShellTimeout is Shell with an explicit deadline.
func (c *Client) ShellTimeout(ctx context.Context, timeout time.Duration, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	ec := exec.CommandContext(ctx, c.Binary, "-s", c.Serial, "shell", cmd)
	var stdout, stderr bytes.Buffer
	ec.Stdout = &stdout
	ec.Stderr = &stderr
	err := ec.Run()
	out := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	if c.Debug {
		fmt.Fprintf(os.Stderr, "[adb %s] %s (%d bytes, %s)\n", c.Serial, cmd, len(out), time.Since(start).Round(time.Millisecond))
	}
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("timed out after %s: %s", timeout, firstLine(cmd))
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(out)
		}
		return out, fmt.Errorf("%s: %s", firstLine(cmd), lastLine(msg))
	}
	return out, nil
}

// Quote single-quotes a string for the device shell (mksh/toybox).
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
