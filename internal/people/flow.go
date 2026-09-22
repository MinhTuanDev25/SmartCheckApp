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
	StepDelay    = 3 * time.Second

	EnableAttendanceAction = true
	EnableConfirmAction    = true

	// Đợi UI load sau PIN / sau mở màn wifi trước khi báo miss.
	UIWaitTimeout = 15 * time.Second
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

	fmt.Printf("[%s] waiting for Chấm công Wifi (up to %s)...\n", action, UIWaitTimeout)
	wifiNeedles := []string{
		WifiMenuLabel,
		WifiScreenTitle,
		"Chấm công WiFi",
		"Cham cong Wifi",
		"cham cong wifi",
	}
	x, y, matched, err := r.adb.WaitAndTapAnyContains(UIWaitTimeout, wifiNeedles...)
	if err != nil {
		r.saveMiss(action, "wifi_menu")
		return fmt.Errorf("open wifi menu: %w", err)
	}
	fmt.Printf("[%s] tapped wifi menu (%q) at (%d, %d)\n", action, matched, x, y)

	checkNeedles := []string{"CHECK-IN", "check-in", "Check-In", "checkin"}
	checkLabel := "CHECK-IN"
	if action == "check-out" {
		checkNeedles = []string{"CHECK-OUT", "check-out", "Check-Out", "checkout"}
		checkLabel = "CHECK-OUT"
	}

	fmt.Printf("[%s] waiting for %s (up to %s)...\n", action, checkLabel, UIWaitTimeout)
	if EnableAttendanceAction {
		x, y, matched, err = r.adb.WaitAndTapAnyContains(UIWaitTimeout, checkNeedles...)
		if err != nil {
			r.saveMiss(action, strings.ToLower(checkLabel))
			return fmt.Errorf("%s button not on wifi screen — abort so next schedule can retry: %w", checkLabel, err)
		}
		fmt.Printf("[%s] tapped %s (%q) at (%d, %d)\n", action, checkLabel, matched, x, y)

		if EnableConfirmAction {
			if err := r.tapConfirm(action); err != nil {
				return err
			}
			r.wait()
		} else {
			fmt.Printf("[%s] skip confirm (disabled)\n", action)
		}
	} else {
		// Dry-run: chỉ cần thấy nút, không tap.
		if err := r.adb.WaitForAnyLabelContains(UIWaitTimeout, checkNeedles...); err != nil {
			r.saveMiss(action, strings.ToLower(checkLabel))
			return fmt.Errorf("%s button not on wifi screen — abort so next schedule can retry: %w", checkLabel, err)
		}
		fmt.Printf("[%s] on wifi attendance screen (found %s) — skip tap (disabled)\n", action, checkLabel)
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
		r.saveMiss(action, "confirm")
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

// saveMiss writes UI XML + PNG screenshot under logs/ for later debugging.
func (r *Runner) saveMiss(action, step string) {
	_ = os.MkdirAll("logs", 0o755)
	stamp := time.Now().Format("150405")
	safeAction := strings.ReplaceAll(action, " ", "_")
	base := fmt.Sprintf("logs/miss_%s_%s_%s", step, safeAction, stamp)

	if dump, err := r.adb.DumpUI(3); err == nil {
		path := base + ".xml"
		if werr := os.WriteFile(path, []byte(dump), 0o644); werr == nil {
			fmt.Printf("[%s] saved miss UI → %s\n", action, path)
		} else {
			fmt.Printf("[%s] save miss UI failed: %v\n", action, werr)
		}
	} else {
		fmt.Printf("[%s] miss UI dump failed: %v\n", action, err)
	}

	if png, err := r.adb.ScreenshotPNG(); err == nil {
		path := base + ".png"
		if werr := os.WriteFile(path, png, 0o644); werr == nil {
			fmt.Printf("[%s] saved miss screenshot → %s\n", action, path)
		} else {
			fmt.Printf("[%s] save miss screenshot failed: %v\n", action, werr)
		}
	} else {
		fmt.Printf("[%s] miss screenshot failed: %v\n", action, err)
	}
}
