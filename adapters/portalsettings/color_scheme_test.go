package portalsettings

import (
	"context"
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockPortalCaller implements portalCaller for tests.
type mockPortalCaller struct {
	callFn func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call
}

func (m mockPortalCaller) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
	return m.callFn(ctx, method, flags, args...)
}

// makeCall builds a *dbus.Call with the given body and error.
func makeCall(body []any, err error) *dbus.Call {
	done := make(chan *dbus.Call, 1)
	c := &dbus.Call{
		Body: body,
		Err:  err,
		Done: done,
	}
	done <- c
	return c
}

// ---------------------------------------------------------------------------
// parseSettingChangedSignal
// ---------------------------------------------------------------------------

func TestParseSettingChangedSignal_Valid(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{
			"org.freedesktop.appearance",
			"color-scheme",
			dbus.MakeVariant(uint32(1)),
		},
	}
	pref, ok, err := parseSettingChangedSignal(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pref != config.SystemColorSchemePreferDark {
		t.Fatalf("got %v, want %v", pref, config.SystemColorSchemePreferDark)
	}
}

func TestParseSettingChangedSignal_Nil(t *testing.T) {
	pref, ok, err := parseSettingChangedSignal(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for nil signal")
	}
	if pref != config.SystemColorSchemeNoPreference {
		t.Fatalf("got %v, want %v", pref, config.SystemColorSchemeNoPreference)
	}
}

func TestParseSettingChangedSignal_WrongBodyLength(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{"a", "b"},
	}
	_, ok, err := parseSettingChangedSignal(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for wrong body length")
	}
}

func TestParseSettingChangedSignal_WrongNamespace(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{
			"org.freedesktop.Wrong",
			"color-scheme",
			dbus.MakeVariant(uint32(1)),
		},
	}
	_, ok, err := parseSettingChangedSignal(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for wrong namespace")
	}
}

func TestParseSettingChangedSignal_WrongKey(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{
			"org.freedesktop.appearance",
			"wrong-key",
			dbus.MakeVariant(uint32(1)),
		},
	}
	_, ok, err := parseSettingChangedSignal(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for wrong key")
	}
}

func TestParseSettingChangedSignal_WrongVariantType(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{
			"org.freedesktop.appearance",
			"color-scheme",
			"not-a-variant",
		},
	}
	_, _, err := parseSettingChangedSignal(signal)
	if err == nil {
		t.Fatal("expected error for non-Variant body element")
	}
}

func TestParseSettingChangedSignal_NegativeValue(t *testing.T) {
	signal := &dbus.Signal{
		Body: []any{
			"org.freedesktop.appearance",
			"color-scheme",
			dbus.MakeVariant(int32(-1)),
		},
	}
	_, _, err := parseSettingChangedSignal(signal)
	if err == nil {
		t.Fatal("expected error for negative int32")
	}
}

// ---------------------------------------------------------------------------
// variantUint32
// ---------------------------------------------------------------------------

func TestVariantUint32_DirectUint32(t *testing.T) {
	v := dbus.MakeVariant(uint32(42))
	got, err := variantUint32(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestVariantUint32_NestedVariant(t *testing.T) {
	v := dbus.MakeVariant(dbus.MakeVariant(uint32(7)))
	got, err := variantUint32(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestVariantUint32_Uint(t *testing.T) {
	v := dbus.MakeVariant(uint(99))
	got, err := variantUint32(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 99 {
		t.Fatalf("got %d, want 99", got)
	}
}

func TestVariantUint32_PositiveInt32(t *testing.T) {
	v := dbus.MakeVariant(int32(5))
	got, err := variantUint32(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestVariantUint32_NegativeInt32(t *testing.T) {
	v := dbus.MakeVariant(int32(-3))
	_, err := variantUint32(v)
	if err == nil {
		t.Fatal("expected error for negative int32")
	}
}

func TestVariantUint32_PositiveInt(t *testing.T) {
	v := dbus.MakeVariant(int(10))
	got, err := variantUint32(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 10 {
		t.Fatalf("got %d, want 10", got)
	}
}

func TestVariantUint32_NegativeInt(t *testing.T) {
	v := dbus.MakeVariant(int(-7))
	_, err := variantUint32(v)
	if err == nil {
		t.Fatal("expected error for negative int")
	}
}

func TestVariantUint32_UnsupportedType(t *testing.T) {
	v := dbus.MakeVariant("hello")
	_, err := variantUint32(v)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// ---------------------------------------------------------------------------
// isUnknownMethodError
// ---------------------------------------------------------------------------

func TestIsUnknownMethodError_True(t *testing.T) {
	err := dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownMethod"}
	if !isUnknownMethodError(err) {
		t.Fatal("expected true for UnknownMethod error")
	}
}

func TestIsUnknownMethodError_OtherDBusError(t *testing.T) {
	err := dbus.Error{Name: "org.freedesktop.DBus.Error.InvalidArgs"}
	if isUnknownMethodError(err) {
		t.Fatal("expected false for non-UnknownMethod DBus error")
	}
}

func TestIsUnknownMethodError_GenericError(t *testing.T) {
	if isUnknownMethodError(errors.New("something went wrong")) {
		t.Fatal("expected false for generic error")
	}
}

func TestIsUnknownMethodError_Nil(t *testing.T) {
	if isUnknownMethodError(nil) {
		t.Fatal("expected false for nil")
	}
}

// ---------------------------------------------------------------------------
// readPortalColorScheme
// ---------------------------------------------------------------------------

func TestReadPortalColorScheme_ReadOneSucceeds(t *testing.T) {
	obj := mockPortalCaller{
		callFn: func(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
			if method != "org.freedesktop.portal.Settings.ReadOne" {
				t.Fatalf("unexpected method: %s", method)
			}
			return makeCall([]any{dbus.MakeVariant(uint32(1))}, nil)
		},
	}
	got, err := readPortalColorScheme(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestReadPortalColorScheme_FallbackToRead(t *testing.T) {
	var callCount int
	obj := mockPortalCaller{
		callFn: func(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
			callCount++
			if method == "org.freedesktop.portal.Settings.ReadOne" {
				return makeCall(nil, dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownMethod"})
			}
			if method == "org.freedesktop.portal.Settings.Read" {
				return makeCall([]any{dbus.MakeVariant(uint32(2))}, nil)
			}
			t.Fatalf("unexpected method: %s", method)
			return nil
		},
	}
	got, err := readPortalColorScheme(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls (ReadOne + Read), got %d", callCount)
	}
}

func TestReadPortalColorScheme_FallbackFails(t *testing.T) {
	obj := mockPortalCaller{
		callFn: func(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
			if method == "org.freedesktop.portal.Settings.ReadOne" {
				return makeCall(nil, dbus.Error{Name: "org.freedesktop.DBus.Error.UnknownMethod"})
			}
			return makeCall(nil, errors.New("secondary failure"))
		},
	}
	_, err := readPortalColorScheme(context.Background(), obj)
	if err == nil {
		t.Fatal("expected error when fallback also fails")
	}
}

func TestReadPortalColorScheme_ReadOneNonUnknownError(t *testing.T) {
	obj := mockPortalCaller{
		callFn: func(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
			if method == "org.freedesktop.portal.Settings.ReadOne" {
				return makeCall(nil, errors.New("connection refused"))
			}
			t.Fatalf("unexpected fallback to Read for non-UnknownMethod error")
			return nil
		},
	}
	_, err := readPortalColorScheme(context.Background(), obj)
	if err == nil {
		t.Fatal("expected error for non-UnknownMethod ReadOne failure")
	}
}

func TestReadPortalColorScheme_ReadOneErrorPreserved(t *testing.T) {
	originalErr := errors.New("dbus: something went wrong")
	obj := mockPortalCaller{
		callFn: func(_ context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
			return makeCall(nil, originalErr)
		},
	}
	_, err := readPortalColorScheme(context.Background(), obj)
	if !errors.Is(err, originalErr) {
		t.Fatalf("expected original error, got %v", err)
	}
}
