package main

import (
	"fmt"
	"os"

	"github.com/appbip/appbip/internal/adb"
	"github.com/appbip/appbip/internal/people"
	"github.com/appbip/appbip/internal/schedule"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	if cmd == "schedule" {
		runner, err := newRunner()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := schedule.Run(runner); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runner, err := newRunner()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "check-in":
		err = runner.CheckIn()
	case "check-out":
		err = runner.CheckOut()
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRunner() (*people.Runner, error) {
	adbBin := os.Getenv("APPBIP_ADB")
	if adbBin == "" {
		adbBin = "adb"
	}
	device := os.Getenv("APPBIP_DEVICE")
	client := adb.New(adbBin, device)
	if device == "" {
		var err error
		device, err = client.FirstDevice()
		if err != nil {
			return nil, err
		}
		client = adb.New(adbBin, device)
	}
	return people.NewRunner(client), nil
}

func usage() {
	fmt.Println(`AppBip - People HDBank chấm công wifi

Usage:
  appbip check-in
  appbip check-out
  appbip schedule

Schedule (giờ VN, Asia/Ho_Chi_Minh, T2/T4/T5/T6; không chấm T3/T7/CN):
  10 lần check-in  random giây 07:50:00 → 08:07:00 (cách nhau ≥ 90s; lần sau bỏ nếu đã OK)
  10 lần check-out random giây 17:00:00 → 17:15:00 (cách nhau ≥ 90s; lần sau bỏ nếu đã OK)
  Mỗi bước trong flow cách nhau 3s
  Failures → logs/YYYY-MM-DD.log

Env:
  APPBIP_ADB     Đường dẫn adb.exe (nếu chưa có trong PATH)
  APPBIP_DEVICE  ADB device serial (optional)
  APPBIP_PIN     PIN mặc định 123456
  APPBIP_TZ      Timezone (mặc định Asia/Ho_Chi_Minh)`)
}
