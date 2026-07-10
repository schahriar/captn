package tui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"golang.org/x/term"
)

const defaultHeight = 2

type Overlay struct {
	cmd  *exec.Cmd
	ptmx *os.File
	out  *os.File

	mu      sync.Mutex
	height  int
	visible bool
	swRow   int
	cols    int

	status []Section
	sub    []Section

	prog *tea.Program
	done chan struct{}
}

func NewOverlay(cmd *exec.Cmd) (*Overlay, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	// banstructlit:ignore
	return &Overlay{
		cmd:     cmd,
		ptmx:    ptmx,
		out:     os.Stdout,
		height:  defaultHeight,
		visible: true,
	}, nil
}

func (o *Overlay) Run() error {
	defer o.ptmx.Close()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			o.resizePty()
			o.setScrollRegion()
		}
	}()
	ch <- syscall.SIGWINCH
	defer signal.Stop(ch)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	o.start()

	go func() {
		_, _ = io.Copy(o.ptmx, os.Stdin)
	}()

	_, _ = io.Copy(o.writer(), o.ptmx)

	o.stop()
	_ = term.Restore(int(os.Stdin.Fd()), oldState)
	return o.cmd.Wait()
}

func (o *Overlay) SetStatus(sections ...Section) {
	o.mu.Lock()
	o.status = sections
	prog := o.prog
	o.mu.Unlock()
	if prog != nil {
		prog.Send(statusMsg(sections))
	}
}

func (o *Overlay) SetSubStatus(sections ...Section) {
	o.mu.Lock()
	o.sub = sections
	prog := o.prog
	o.mu.Unlock()
	if prog != nil {
		prog.Send(subStatusMsg(sections))
	}
}

func (o *Overlay) Show() {
	o.mu.Lock()
	changed := !o.visible
	o.visible = true
	height := o.height
	o.mu.Unlock()
	if changed {
		o.applyLayout(height)
	}
}

func (o *Overlay) Hide() {
	o.mu.Lock()
	changed := o.visible
	o.visible = false
	height := o.height
	o.mu.Unlock()
	if changed {
		o.applyLayout(height)
	}
}

func (o *Overlay) SetHeight(rows int) {
	if rows < 0 {
		rows = 0
	}

	o.mu.Lock()
	changed := rows != o.height
	prev := o.height
	o.height = rows
	o.mu.Unlock()
	if changed {
		o.applyLayout(max(prev, rows))
	}
}

func (o *Overlay) applyLayout(clearRows int) {
	o.clearBottomRows(clearRows)
	o.setScrollRegion()
	o.resizePty()
}

func (o *Overlay) clearBottomRows(height int) {
	_, rows, err := term.GetSize(int(o.out.Fd()))
	if err != nil || height <= 0 {
		return
	}
	top := rows - height + 1
	if top < 1 {
		top = 1
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = fmt.Fprintf(o.out, ansiMoveRowFmt+ansiEraseBelow, top)
}

func (o *Overlay) reservedRows() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.visible {
		return 0
	}
	return o.height
}

func (o *Overlay) start() {
	o.clearScreen()
	o.setScrollRegion()
	o.startProgram()
}

func (o *Overlay) stop() {
	o.mu.Lock()
	prog := o.prog
	done := o.done
	o.prog = nil
	o.mu.Unlock()
	if prog != nil {
		prog.Quit()
		<-done
	}
	o.resetScrollRegion()
}

func (o *Overlay) resizePty() {
	cols, rows, err := term.GetSize(int(o.out.Fd()))
	if err != nil {
		return
	}

	reserved := o.reservedRows()
	h := rows - reserved
	if h < 1 {
		h = 1
	}

	_ = pty.Setsize(o.ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(cols), Y: uint16(reserved)})
	if o.cmd.Process != nil {
		_ = o.cmd.Process.Signal(syscall.SIGWINCH)
	}
}

func (o *Overlay) clearScreen() {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = io.WriteString(o.out, ansiClearScreen)
}

func (o *Overlay) setScrollRegion() {
	cols, rows, err := term.GetSize(int(o.out.Fd()))
	if err != nil {
		return
	}

	reserved := o.reservedRows()
	bottom := rows - reserved // last row the child may use
	if bottom < 1 {
		bottom = 1
	}
	swRow := bottom + 1

	var b bytes.Buffer
	fmt.Fprintf(&b, ansiScrollRegionFmt, bottom) // confine the child to the top rows
	if reserved >= 2 {
		fmt.Fprintf(&b, ansiMoveRowFmt+"%s", swRow, strings.Repeat("─", cols))
		swRow++
	}
	fmt.Fprintf(&b, ansiMoveRowFmt, bottom) // park the cursor back in the child's region

	o.mu.Lock()
	o.swRow = swRow
	o.cols = cols
	prog := o.prog
	_, _ = o.out.Write(b.Bytes())
	o.mu.Unlock()

	if prog != nil {
		prog.Send(widthMsg(cols))
	}
}

func (o *Overlay) resetScrollRegion() {
	_, rows, err := term.GetSize(int(o.out.Fd()))
	if err != nil {
		rows = 1
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = fmt.Fprintf(o.out, ansiResetRegion+ansiMoveRowFmt, rows)
}

func (o *Overlay) startProgram() {
	pr, pw := io.Pipe()

	o.mu.Lock()
	// banstructlit:ignore
	m := barModel{status: o.status, sub: o.sub, width: o.cols}
	o.mu.Unlock()

	p := tea.NewProgram(
		m,
		tea.WithInput(pr),
		// banstructlit:ignore
		tea.WithOutput(&regionWriter{o: o}),
		tea.WithoutSignalHandler(),
	)

	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		_ = pw.Close()
		close(done)
	}()

	o.mu.Lock()
	o.prog = p
	o.done = done
	o.mu.Unlock()
}

func (o *Overlay) writer() io.Writer {
	// banstructlit:ignore
	return &lockedWriter{mu: &o.mu, out: o.out}
}

type statusMsg []Section

type subStatusMsg []Section

type widthMsg int

type shimmerTickMsg struct{}

type barModel struct {
	status []Section
	sub    []Section
	phase  float64
	width  int
}

func shimmerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		// banstructlit:ignore
		return shimmerTickMsg{}
	})
}

func (m barModel) Init() tea.Cmd { return shimmerTick() }

func (m barModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.status = []Section(msg)
		return m, nil
	case subStatusMsg:
		m.sub = []Section(msg)
		return m, nil
	case widthMsg:
		m.width = int(msg)
		return m, nil
	case shimmerTickMsg:
		m.phase = advanceShimmer(m.phase)
		return m, shimmerTick()
	}

	return m, nil
}

func (m barModel) View() string {
	status := renderSections(m.status)
	sub := renderSections(m.sub)
	if status == "" && sub == "" {
		return ""
	}

	line := status
	if sub != "" {
		// Right-align the sub status, leaving the last column free to avoid wrap.
		gap := m.width - lineWidth(status) - lineWidth(sub) - 1
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + sub
	}
	return renderShimmer(line, m.phase) + ansiReset
}

func renderSections(sections []Section) string {
	var b strings.Builder
	for _, s := range sections {
		if s == nil {
			continue
		}

		b.WriteString(s.Render())
	}
	return b.String()
}

func lineWidth(s string) int {
	w := 0
	for _, p := range splitShimmer(s) {
		w += ansi.StringWidth(p.text)
	}
	return w
}

type lockedWriter struct {
	mu  *sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

type regionWriter struct {
	o *Overlay
}

func (w *regionWriter) Write(p []byte) (int, error) {
	frame := bytes.ReplaceAll(p, []byte(ansiHideCursor), nil)
	frame = bytes.ReplaceAll(frame, []byte(ansiShowCursor), nil)

	w.o.mu.Lock()
	defer w.o.mu.Unlock()

	if !w.o.visible || w.o.height < 2 {
		return len(p), nil
	}

	var b bytes.Buffer
	b.WriteString(ansiSaveCursor)
	fmt.Fprintf(&b, ansiMoveRowFmt, w.o.swRow)
	b.WriteString(ansiClearLine)
	b.Write(frame)
	b.WriteString(ansiRestoreCursor)
	_, err := w.o.out.Write(b.Bytes())
	return len(p), err
}
