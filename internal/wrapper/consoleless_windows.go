//go:build windows && windowsgui

package wrapper

// createNoWindow is CREATE_NO_WINDOW from the Windows process creation flags.
const createNoWindow uint32 = 0x08000000

// creationFlags carries CREATE_NO_WINDOW in the GUI-subsystem build. This
// variant has no console of its own, so a console child would otherwise be
// given a fresh one, flashing a window on screen. The standard handles are
// still passed through, so redirected output keeps flowing.
const creationFlags = createNoWindow
