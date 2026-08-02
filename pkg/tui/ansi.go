package tui

const (
	ansiClearScreen    = "\x1b[2J\x1b[H"
	ansiClearLine      = "\x1b[2K"
	ansiEraseBelow     = "\x1b[J"
	ansiResetRegion    = "\x1b[r"
	ansiSaveCursor     = "\x1b7"
	ansiRestoreCursor  = "\x1b8"
	ansiBold           = "\x1b[1m"
	ansiDim            = "\x1b[2m"
	ansiIntensityOff   = "\x1b[22m"
	ansiFgReset        = "\x1b[39m"
	ansiReset          = "\x1b[0m"
	ansiHideCursor     = "\x1b[?25l"
	ansiShowCursor     = "\x1b[?25h"
	ansiEnterAltScreen = "\x1b[?1049h"
	ansiExitAltScreen  = "\x1b[?1049l"

	ansiScrollRegionFmt = "\x1b[1;%dr"
	ansiRegionPairFmt   = "\x1b[%d;%dr"
	ansiMoveRowFmt      = "\x1b[%d;1H"
	ansiFgTrueColorFmt  = "\x1b[38;2;%d;%d;%dm"
)
