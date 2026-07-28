package ui

import "time"

// toggleGrace is how long after the panel closed itself a tray click still
// counts as the click that closed it, rather than a fresh request to open.
//
// Clicking the tray icon can take focus away from the panel, and the panel
// closes on focus loss, so the close can land BEFORE the host delivers the
// click. Without this the second click would reopen the panel instead of
// dismissing it. Long enough to cover that ordering, short enough that a
// deliberate reopen a moment later still works.
const toggleGrace = 400 * time.Millisecond
