package people

import (
	"fmt"
	"os"
	"time"

	"github.com/appbip/appbip/internal/adb"
)

const (
	Package    = "com.people.fis.hdbank.pro"
	DefaultPIN = "123456"

	ScreenWidth  = 1320
	ScreenHeight = 2112
	StepDelay    = 5 * time.Second

	EnableAttendanceAction = true
	EnableConfirmAction    = true
)

const AppLabel = "People HDBank"

const (
	WifiMenuLabel   = "Chấm công Wifi"  // nút trên màn hình chính app
	WifiScreenTitle = "Chấm công WIFI"  // tiêu đề màn hình chấm công
	PINFieldLabel   = "Nhập mã pin"
	ConfirmLabel    = "XÁC NHẬN"
)

type Runner struct {
	adb *adb.Client
}

func NewRunner(client *adb.Client) *Runner {
	return &Runner{adb: client}
}

func (r *Runner) CheckIn() error {
	return r.run("check-in")
}

func (r *Runner) CheckOut() error {
	return r.run("check-out")
}

func (r *Runner) run(action string) (err error) {
	defer func() {
		if err != nil {
			r.emergencyCleanup(action, err)
		}
	}()

	fmt.Printf("[%s] waking tablet...\n", action)
	if err := r.adb.Wake(); err != nil {
		return err
	}

	fmt.Printf("[%s] swipe up (lần 1) — mở khóa...\n", action)
	if err := r.adb.SwipeUpUnlock(ScreenWidth, ScreenHeight); err != nil {
		return err
	}
	r.wait()

	visible, err := r.adb.HasLabel(AppLabel)
	if err != nil {
		return fmt.Errorf("check screen: %w", err)
	}
	if visible {
		fmt.Printf("[%s] skip swipe up (lần 2) — đã thấy %s\n", action, AppLabel)
	} else {
		fmt.Printf("[%s] swipe up (lần 2) — về trang app...\n", action)
		if err := r.adb.SwipeUpHome(ScreenWidth, ScreenHeight); err != nil {
			return err
		}
		r.wait()
	}

	fmt.Printf("[%s] finding %s...\n", action, AppLabel)
	x, y, err := r.adb.TapByLabel(AppLabel)
	if err != nil {
		return fmt.Errorf("open app: %w", err)
	}
	fmt.Printf("[%s] tapped %s at (%d, %d)\n", action, AppLabel, x, y)
	r.wait()

	fmt.Printf("[%s] entering PIN...\n", action)
	pin := os.Getenv("APPBIP_PIN")
	if pin == "" {
		pin = DefaultPIN
	}
	onPIN, err := r.adb.HasLabel(PINFieldLabel)
	if err != nil {
		return fmt.Errorf("check PIN screen: %w", err)
	}
	if !onPIN {
		return fmt.Errorf("PIN screen not shown (missing %q)", PINFieldLabel)
	}
	x, y, err = r.adb.TapByLabelContains(PINFieldLabel)
	if err != nil {
		return fmt.Errorf("focus PIN field: %w", err)
	}
	fmt.Printf("[%s] tapped PIN field at (%d, %d)\n", action, x, y)
	r.adb.Sleep(500 * time.Millisecond)
	if err := r.adb.Text(pin); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] opening Chấm công Wifi...\n", action)
	x, y, err = r.adb.TapByLabelContains(WifiMenuLabel)
	if err != nil {
		return fmt.Errorf("open wifi menu: %w", err)
	}
	fmt.Printf("[%s] tapped %s at (%d, %d)\n", action, WifiMenuLabel, x, y)
	r.wait()

	onWifi, err := r.adb.HasAnyLabel(WifiScreenTitle, "CHECK-IN", "CHECK-OUT")
	if err != nil {
		return fmt.Errorf("verify wifi screen: %w", err)
	}
	if !onWifi {
		return fmt.Errorf("wifi attendance screen not opened (expected %q or CHECK-IN/CHECK-OUT)", WifiScreenTitle)
	}
	fmt.Printf("[%s] on wifi attendance screen\n", action)

	if EnableAttendanceAction {
		checkLabel := "CHECK-IN"
		if action == "check-out" {
			checkLabel = "CHECK-OUT"
		}
		fmt.Printf("[%s] tapping %s...\n", action, checkLabel)
		if _, _, err := r.adb.TapByLabel(checkLabel); err != nil {
			return fmt.Errorf("tap %s: %w", checkLabel, err)
		}
		r.wait()

		if EnableConfirmAction {
			fmt.Printf("[%s] confirming popup...\n", action)
			if _, _, err := r.adb.TapByLabelContains(ConfirmLabel); err != nil {
				return fmt.Errorf("tap confirm: %w", err)
			}
			r.wait()
		} else {
			fmt.Printf("[%s] skip confirm (disabled)\n", action)
		}
	} else {
		fmt.Printf("[%s] skip %s (disabled)\n", action, action)
	}

	fmt.Printf("[%s] force stopping app...\n", action)
	if err := r.adb.ForceStop(Package); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] opening recents...\n", action)
	if err := r.adb.Home(); err != nil {
		return err
	}
	r.wait()
	if err := r.adb.Tap(adb.RecentsButtonX, adb.RecentsButtonY); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] removing app from recents...\n", action)
	if err := r.adb.Swipe(adb.RecentsCardSwipeFromX, adb.RecentsCardSwipeFromY, adb.RecentsCardSwipeFromX, adb.RecentsCardSwipeToY, 400); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] back to home...\n", action)
	if err := r.adb.Home(); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] putting tablet to sleep...\n", action)
	if err := r.adb.ScreenOff(); err != nil {
		return err
	}

	fmt.Printf("[%s] done\n", action)
	return nil
}

// emergencyCleanup force-stops the app and sleeps the tablet after any failed step.
// Best-effort: always tries to reset device state before the next scheduled run.
func (r *Runner) emergencyCleanup(action string, cause error) {
	fmt.Printf("[%s] failed: %v\n", action, cause)
	fmt.Printf("[%s] emergency cleanup — force stop app...\n", action)
	_ = r.adb.ForceStop(Package)
	_ = r.adb.Home()
	fmt.Printf("[%s] emergency cleanup — putting tablet to sleep...\n", action)
	_ = r.adb.ScreenOff()
	fmt.Printf("[%s] cleanup done — will retry at next scheduled time\n", action)
}

func (r *Runner) wait() {
	r.adb.Sleep(StepDelay)
}
