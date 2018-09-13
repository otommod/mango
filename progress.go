package main

import (
	"fmt"
	"image/color"
)

type Task int64

type progress struct {
	task  Task
	sofar int64
	total int64
}

func (p *progress) Tick(currentProgress int64) {
	p.sofar = currentProgress
}

type ProgressBar struct {
	gradient LinearGradient
	startCh  chan Task
	tickCh   chan progress
	stopCh   chan empty
	stopped  chan empty
}

func NewProgressBar() *ProgressBar {
	gradient := LinearGradient{
		color.RGBA{192, 3, 20, 255},
		color.RGBA{255, 255, 0, 255},
		color.RGBA{3, 192, 20, 255},
	}

	p := &ProgressBar{
		gradient: gradient,
		startCh:  make(chan Task),
		tickCh:   make(chan progress),
		stopCh:   make(chan empty),
		stopped:  make(chan empty),
	}
	go p.run()
	return p
}

func (p ProgressBar) NewTask() Task {
	newTask := <-p.startCh
	p.TickTask(newTask, 0, 0)
	return newTask
}

func (p ProgressBar) TickTask(task Task, sofar, total int64) {
	p.tickCh <- progress{task, sofar, total}
}

func (p ProgressBar) run() {
	fmt.Print("\033[?25l")       // cursor off
	defer fmt.Print("\033[?25h") // cursor on

	// This is because the escape code that places the cursor, at least on my
	// terminal, treats the zeroth and the first place as the same, so you'd
	// have some overlapping tasks.
	var nextPlace Task = 1

	chars := []string{"▁", "▃", "▄", "▅", "▆", "▇", "█"}

loop:
	for {
		select {
		case <-p.stopCh:
			break loop

		case p.startCh <- nextPlace:
			nextPlace++

		case progress := <-p.tickCh:
			var color int
			var char string
			if progress.total <= 0 {
				color = 7 // white/grey
				char = chars[len(chars)-1]
			} else {
				percent := float64(progress.sofar) / float64(progress.total)
				color = XTerm256Palette.Index(p.gradient.At(percent))
				char = chars[int(percent*float64(len(chars)-1))]
			}
			fmt.Printf("\033[%dG\033[38;5;%dm%s\033[0m", progress.task, color, char)
		}
	}
	close(p.stopped)
}

func (p ProgressBar) Stop() {
	close(p.stopCh)
	<-p.stopped
}
