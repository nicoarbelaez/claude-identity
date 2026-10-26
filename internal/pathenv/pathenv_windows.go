//go:build windows

package pathenv

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type userPath struct{}

// New returns an Editor backed by HKCU\Environment.
func New() Editor { return userPath{} }

func (userPath) RemoveDir(dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("open user environment: %w", err)
	}
	defer func() { _ = k.Close() }()

	// GetStringValue does not expand %VAR%, so other entries stay as written.
	raw, kind, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read user PATH: %w", err)
	}
	next, removed := removePathEntry(raw, dir)
	if !removed {
		return false, nil
	}
	if kind == registry.EXPAND_SZ {
		err = k.SetExpandStringValue("Path", next)
	} else {
		err = k.SetStringValue("Path", next)
	}
	if err != nil {
		return false, fmt.Errorf("write user PATH: %w", err)
	}
	broadcastEnvChange()
	return true, nil
}

// broadcastEnvChange tells running programs the environment changed. Best
// effort: failures are ignored because the registry write already succeeded.
func broadcastEnvChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)),
		smtoAbortIfHung, 2000, uintptr(unsafe.Pointer(&result)))
}
