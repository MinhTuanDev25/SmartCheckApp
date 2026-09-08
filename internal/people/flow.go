package people

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/appbip/appbip/internal/adb"
)

const (
	Package    = "com.people.fis.hdbank.pro"
	DefaultPIN = "123456"

	// Samsung SM-P619 (Galaxy Tab S6 Lite 2022)
	ScreenWidth  = 1200
	ScreenHeight = 2000
	StepDelay    = 5 * time.Second

	EnableAttendanceAction = true
	EnableConfirmAction    = true
)

const AppLabel = "People HDBank"

const (
	WifiMenuLabel   = "Chấm công Wifi" // nút trên màn hình chính app
	WifiScreenTitle = "Chấm công WIFI" // tiêu đề màn hình chấm công
	PINFieldLabel   = "Nhập mã pin"
	ConfirmLabel    = "XÁC NHẬN"
)

// Fallback tọa độ (Samsung SM-P619 1200×2000; scaled from SM-X406B dump).
var (
	CheckInButton  = [2]int{324, 715}
	CheckOutButton = [2]int{876, 715}
	// Center of popup button bounds [886,1019][1037,1100] from live dump.
	ConfirmButton = [2]int{961, 1059}
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

	checkLabel := "CHECK-IN"
	checkSubstr := "check-in"
	if action == "check-out" {
		checkLabel = "CHECK-OUT"
		checkSubstr = "check-out"
	}

	// Bắt buộc thấy nút CHECK-IN/OUT — không chỉ thấy tiêu đề wifi rồi chạy tiếp.
	hasBtn, err := r.adb.HasAnyLabelContains(checkSubstr, strings.ToLower(checkLabel))
	if err != nil {
		return fmt.Errorf("verify wifi screen: %w", err)
	}
	if !hasBtn {
		return fmt.Errorf("%s button not on wifi screen — abort so next schedule can retry", checkLabel)
	}
	fmt.Printf("[%s] on wifi attendance screen (found %s)\n", action, checkLabel)

	if EnableAttendanceAction {
		fmt.Printf("[%s] tapping %s...\n", action, checkLabel)
		x, y, err := r.adb.TapByLabel(checkLabel)
		if err != nil {
			x, y, err = r.adb.TapByLabelContains(checkSubstr)
		}
		if err != nil {
			return fmt.Errorf("tap %s failed (no fallback coords): %w", checkLabel, err)
		}
		fmt.Printf("[%s] tapped %s at (%d, %d)\n", action, checkLabel, x, y)
		r.wait()

		if EnableConfirmAction {
			if err := r.tapConfirm(action); err != nil {
				return err
			}
			r.wait()
		} else {
			fmt.Printf("[%s] skip confirm (disabled)\n", action)
		}
	} else {
		fmt.Printf("[%s] skip %s (disabled)\n", action, action)
	}

	fmt.Printf("[%s] force stopping app...\n", action)
	if err := r.resetToCleanState(action); err != nil {
		return err
	}

	fmt.Printf("[%s] done\n", action)
	return nil
}

// emergencyCleanup resets tablet to a clean idle state after any failed step.
// Schedule keeps running — next job will try again. Never leaves People app open.
func (r *Runner) emergencyCleanup(action string, cause error) {
	fmt.Printf("[%s] step skipped: %v\n", action, cause)
	fmt.Printf("[%s] cleanup — về Home + xóa đa nhiệm, chờ lần schedule sau...\n", action)
	_ = r.resetToCleanState(action)
	fmt.Printf("[%s] cleanup done — schedule sẽ chạy tiếp bình thường\n", action)
}

// resetToCleanState: force-stop → Home → ||| → Đóng tất cả → Home → sleep.
func (r *Runner) resetToCleanState(action string) error {
	_ = r.adb.ForceStop(Package)
	r.adb.Sleep(1 * time.Second)

	if err := r.adb.Home(); err != nil {
		return err
	}
	r.wait()

	fmt.Printf("[%s] opening recents (tap |||)...\n", action)
	if err := r.adb.OpenRecentsByNavTap(); err != nil {
		_ = r.adb.OpenRecents()
	}
	r.wait()

	fmt.Printf("[%s] clearing recents...\n", action)
	_ = r.clearRecents(action)
	r.wait()

	fmt.Printf("[%s] back to home...\n", action)
	_ = r.adb.Home()
	r.wait()

	fmt.Printf("[%s] putting tablet to sleep...\n", action)
	_ = r.adb.ScreenOff()
	return nil
}

func (r *Runner) wait() {
	r.adb.Sleep(StepDelay)
}

// clearRecents dismisses overview apps on SM-P619 One UI.
// Prefer "Đóng tất cả"; else swipe the card area upward.
func (r *Runner) clearRecents(action string) error {
	if x, y, err := r.adb.TapByLabel("Đóng tất cả"); err == nil {
		fmt.Printf("[%s] tapped Đóng tất cả at (%d, %d)\n", action, x, y)
		return nil
	}
	if x, y, err := r.adb.TapByLabelContains("Đóng tất cả"); err == nil {
		fmt.Printf("[%s] tapped Đóng tất cả (contains) at (%d, %d)\n", action, x, y)
		return nil
	}
	if x, y, err := r.adb.TapByLabelContains("Clear all"); err == nil {
		fmt.Printf("[%s] tapped Clear all at (%d, %d)\n", action, x, y)
		return nil
	}

	// Fallback: swipe People HDBank card region up (SM-P619 card ~[636,259][975,835]).
	fmt.Printf("[%s] Đóng tất cả not found — swipe card up\n", action)
	return r.adb.Swipe(805, 650, 805, 80, 450)
}

func (r *Runner) tapConfirm(action string) error {
	fmt.Printf("[%s] waiting for confirm popup...\n", action)
	r.adb.Sleep(3 * time.Second)

	if err := r.adb.WaitForConfirmPopup(15 * time.Second); err != nil {
		if dump, dumpErr := r.adb.DumpUI(3); dumpErr == nil {
			_ = os.MkdirAll("logs", 0o755)
			path := fmt.Sprintf("logs/confirm_miss_%s.xml", time.Now().Format("150405"))
			_ = os.WriteFile(path, []byte(dump), 0o644)
			fmt.Printf("[%s] saved miss dump → %s\n", action, path)
		}
		return fmt.Errorf("confirm popup not shown after attendance tap — abort: %w", err)
	}

	// Popup "Thông báo / XÁC NHẬN" hiện = chấm công đã thành công; chỉ cần tap để đóng.
	btnX, btnY, hasBtn, err := r.adb.FindConfirmButton()
	if err != nil {
		return fmt.Errorf("find confirm button: %w", err)
	}
	if hasBtn {
		fmt.Printf("[%s] tapping confirm at (%d, %d)\n", action, btnX, btnY)
		if err := r.adb.Tap(btnX, btnY); err != nil {
			return fmt.Errorf("tap confirm: %w", err)
		}
	} else {
		fmt.Printf("[%s] tapping confirm fallback at (%d, %d)\n", action, ConfirmButton[0], ConfirmButton[1])
		if err := r.adb.Tap(ConfirmButton[0], ConfirmButton[1]); err != nil {
			return fmt.Errorf("tap confirm fallback: %w", err)
		}
	}
	fmt.Printf("[%s] confirm popup shown — treated as success\n", action)
	return nil
}
