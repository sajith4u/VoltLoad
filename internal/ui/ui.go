package ui

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
)

func ShowMetrics(success, failed *int, total int) {
	app := tview.NewApplication()
	text := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	text.SetBorder(true).SetTitle("Real-Time Metrics")

	go func() {
		for *success+*failed < total {
			text.SetText(
				fmt.Sprintf("[green]Success:[white] %d  [red]Failed:[white] %d", *success, *failed),
			)
			time.Sleep(500 * time.Millisecond)
		}
		app.Stop()
	}()

	if err := app.SetRoot(text, true).Run(); err != nil {
		panic(err)
	}
}
