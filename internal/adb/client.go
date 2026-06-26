package adb

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	binary string
	device string
}

func New(binary, device string) *Client {
	if binary == "" {
		binary = "adb"
	}
	return &Client{binary: binary, device: device}
}

func (c *Client) FirstDevice() (string, error) {
	out, err := c.run("devices")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) >= 2 && parts[1] == "device" {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no adb device found")
}

func (c *Client) Swipe(x1, y1, x2, y2, durationMs int) error {
	if durationMs <= 0 {
		durationMs = 400
	}
	_, err := c.deviceCmd("shell", "input", "swipe",
		itoa(x1), itoa(y1), itoa(x2), itoa(y2), itoa(durationMs))
	return err
}

func (c *Client) KeyEvent(code int) error {
	_, err := c.deviceCmd("shell", "input", "keyevent", itoa(code))
	return err
}

func (c *Client) Home() error {
	return c.KeyEvent(3) // KEYCODE_HOME
}

const screenOnDelay = 1200 * time.Millisecond

// Wake turns the screen on from sleep/doze (Samsung tablet).
func (c *Client) Wake() error {
	_ = c.KeyEvent(26)  // KEYCODE_POWER — bật màn hình
	c.Sleep(400 * time.Millisecond)
	_ = c.KeyEvent(224) // KEYCODE_WAKEUP
	c.Sleep(screenOnDelay)
	return nil
}

// ScreenOff puts the tablet to sleep (turns screen off).
func (c *Client) ScreenOff() error {
	return c.KeyEvent(26) // KEYCODE_POWER
}

// SwipeUpUnlock swipes on the lock screen to wake into the launcher.
func (c *Client) SwipeUpUnlock(screenWidth, screenHeight int) error {
	midX := screenWidth / 2
	fromY := screenHeight - 50
	toY := screenHeight / 5
	if err := c.Swipe(midX, fromY, midX, toY, 800); err != nil {
		return err
	}
	c.Sleep(400 * time.Millisecond)
	_, _ = c.deviceCmd("shell", "wm", "dismiss-keyguard")
	return nil
}

// SwipeUpHome swipes on the launcher to the page with app icons.
func (c *Client) SwipeUpHome(screenWidth, screenHeight int) error {
	midX := screenWidth / 2
	fromY := screenHeight * 17 / 20 // ~1795 on 2112px
	toY := screenHeight * 3 / 10    // ~633
	return c.Swipe(midX, fromY, midX, toY, 500)
}

func (c *Client) Tap(x, y int) error {
	_, err := c.deviceCmd("shell", "input", "tap", itoa(x), itoa(y))
	return err
}

func (c *Client) Text(text string) error {
	escaped := strings.ReplaceAll(text, " ", "%s")
	_, err := c.deviceCmd("shell", "input", "text", escaped)
	return err
}

func (c *Client) Launch(pkg, activity string) error {
	_, err := c.deviceCmd("shell", "am", "start", "-n", pkg+"/"+activity)
	return err
}

const (
	RecentsButtonX = 960
	RecentsButtonY = 2076
	// People HDBank card in recents [712,269][1095,891]
	RecentsCardSwipeFromX = 903
	RecentsCardSwipeFromY = 800
	RecentsCardSwipeToY   = 150
)

func (c *Client) ForceStop(pkg string) error {
	_, err := c.deviceCmd("shell", "am", "force-stop", pkg)
	return err
}

func (c *Client) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (c *Client) deviceCmd(args ...string) (string, error) {
	all := args
	if c.device != "" {
		all = append([]string{"-s", c.device}, args...)
	}
	cmd := exec.Command(c.binary, all...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command(c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
