package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Mọi lệnh adb đều có hạn — tránh treo cả schedule khi cáp USB rớt giữa lệnh.
const commandTimeout = 60 * time.Second

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
	_ = c.KeyEvent(26) // KEYCODE_POWER — bật màn hình
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

// Navigation / recents coords for Samsung SM-P619 (1200×2000).
// Old SM-X406B (1320×2112) values scaled + KEYCODE_APP_SWITCH as primary open.
const (
	RecentsButtonX = 872
	RecentsButtonY = 1966
	// Swipe up to dismiss the foreground card in Recents.
	RecentsCardSwipeFromX = 600
	RecentsCardSwipeFromY = 1100
	RecentsCardSwipeToY   = 200
)

// OpenRecents opens the ||| Recents / overview screen.
// Prefer KEYCODE_APP_SWITCH (187); fall back to tapping the nav ||| button.
func (c *Client) OpenRecents() error {
	if err := c.KeyEvent(187); err != nil { // KEYCODE_APP_SWITCH
		return err
	}
	c.Sleep(800 * time.Millisecond)
	return nil
}

// OpenRecentsByNavTap taps the on-screen ||| button (3-button navigation).
func (c *Client) OpenRecentsByNavTap() error {
	return c.Tap(RecentsButtonX, RecentsButtonY)
}

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
	return c.exec(args, all)
}

func (c *Client) run(args ...string) (string, error) {
	return c.exec(args, args)
}

// exec runs adb with a timeout; label is what shows up in error messages.
func (c *Client) exec(label, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("adb %s: timeout sau %s", strings.Join(label, " "), commandTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("adb %s: %w: %s", strings.Join(label, " "), err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
