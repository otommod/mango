package main

import (
	"fmt"
	"image/color"
)

type progress struct {
	place int
	sofar int64
	total int64
}

func (p *progress) Tick(currentProgress int64) {
	p.sofar = currentProgress
}

type ProgressBar struct {
	gradient LinearGradient
	startCh  chan int
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
		startCh:  make(chan int),
		tickCh:   make(chan progress),
		stopCh:   make(chan empty),
		stopped:  make(chan empty),
	}
	go p.run()
	return p
}

func (self ProgressBar) StartTask(total int64) progress {
	newTask := progress{
		<-self.startCh,
		0,
		total,
	}
	self.TickTask(newTask, 0)
	return newTask
}

func (self ProgressBar) TickTask(info progress, sofar int64) {
	info.sofar = sofar
	self.tickCh <- info
}

func (self ProgressBar) run() {
	fmt.Print("\033[?25l")       // cursor off
	defer fmt.Print("\033[?25h") // cursor on

	// This is because the escape code that places the cursor, at least on my
	// terminal, treats the zeroth and the first place as the same, so you'd
	// have some overlapping tasks.
	var nextPlace int = 1

loop:
	for {
		select {
		case <-self.stopCh:
			break loop

		case self.startCh <- nextPlace:
			nextPlace++

		case task := <-self.tickCh:
			var color int
			if task.total <= 0 {
				color = 7 // white/grey
			} else {
				percent := float64(task.sofar) / float64(task.total)
				color = XTerm256Palette.Index(self.gradient.At(percent))
			}
			fmt.Printf("\033[%dG\033[48;5;%dm \033[0m", task.place, color)
		}
	}
	close(self.stopped)
}

func (self ProgressBar) Stop() {
	self.stopCh <- empty{}
	<-self.stopped
}
