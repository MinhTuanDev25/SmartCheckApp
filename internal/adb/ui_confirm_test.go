package adb

import "testing"

func TestIsConfirmButtonText_DecomposedUnicode(t *testing.T) {
	// Exact string from SM-P619 People HDBank dump after CHECK-OUT.
	decomposed := "XA" + "\u0301" + "C NH\u00C2\u0323N" // XÁC NHẬN
	if !isConfirmButtonText(decomposed) {
		t.Fatalf("expected match for decomposed %q (fold=%q)", decomposed, foldVN(decomposed))
	}
	if !isConfirmButtonText("XÁC NHẬN") {
		t.Fatalf("expected match for NFC XÁC NHẬN (fold=%q)", foldVN("XÁC NHẬN"))
	}
	if !isConfirmButtonText("xác nhận") {
		t.Fatal("expected match for lowercase NFC")
	}
	if isConfirmButtonText("CHECK-OUT") {
		t.Fatal("CHECK-OUT must not match confirm")
	}
}

func TestFindConfirmButtonCenter_FromDump(t *testing.T) {
	// Button text uses the same decomposed form as the live SM-P619 dump.
	btn := "XA" + "\u0301" + "C NH\u00C2\u0323N"
	dump := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="Thông báo" resource-id="" class="android.widget.TextView" package="com.people.fis.hdbank.pro" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[180,885][1019,929]" />
  <node index="1" text="Bạn đã Check-Out thành công" resource-id="" class="android.widget.TextView" package="com.people.fis.hdbank.pro" content-desc="" checkable="false" checked="false" clickable="false" enabled="true" focusable="false" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[144,941][1055,977]" />
  <node index="2" text="` + btn + `" resource-id="" class="android.widget.Button" package="com.people.fis.hdbank.pro" content-desc="" checkable="false" checked="false" clickable="true" enabled="true" focusable="true" focused="false" scrollable="false" long-clickable="false" password="false" selected="false" bounds="[886,1019][1037,1100]" />
</hierarchy>`
	x, y, ok := findConfirmButtonCenter(dump)
	if !ok {
		t.Fatal("confirm button not found in dump")
	}
	if x != 961 || y != 1059 {
		t.Fatalf("got (%d,%d), want (961,1059)", x, y)
	}
}
