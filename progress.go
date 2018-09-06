package main

import (
	"fmt"
	"image/color"
	"sync"
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
	start    chan int
	tick     chan progress
	quit     chan empty
	wg       *sync.WaitGroup
}

func NewProgressBar() *ProgressBar {
	p := &ProgressBar{
		gradient: LinearGradient{
			color.RGBA{192, 3, 20, 255},
			color.RGBA{255, 255, 0, 255},
			color.RGBA{3, 192, 20, 255},
		},

		start: make(chan int),
		tick:  make(chan progress),
		quit:  make(chan empty),
		wg:    &sync.WaitGroup{},
	}
	p.wg.Add(1)
	go p.run()
	return p
}

func (p *ProgressBar) StartTask(total int64) progress {
	newTask := progress{
		<-p.start,
		0,
		total,
	}
	p.TickTask(newTask, 0)
	return newTask
}

func (p *ProgressBar) TickTask(info progress, sofar int64) {
	info.sofar = sofar
	p.tick <- info
}

func (p *ProgressBar) run() {
	fmt.Print("\033[?25l")       // cursor off
	defer fmt.Print("\033[?25h") // cursor on

	// This is because the escape code that places the cursor, at least on my
	// terminal, treats the zeroth and the first place as the same, so you'd
	// have some overlapping tasks.
	nextPlace := 1

	defer p.wg.Done()
	// loop:
	for {
		select {
		case <-p.quit:
			return
			// break loop

		case p.start <- nextPlace:
			nextPlace++

		case task := <-p.tick:
			var color int
			if task.total <= 0 {
				color = 7 // white/grey
			} else {
				percent := float64(task.sofar) / float64(task.total)
				color = XTerm256Palette.Index(p.gradient.At(percent))
			}
			fmt.Printf("\0337\033[%dG\033[48;5;%dm \033[0m\0338", task.place, color)
		}
	}
}

func (p *ProgressBar) Stop() {
	p.quit <- empty{}
	fmt.Println("ProgressBar.Stop(): waiting")
	p.wg.Wait()
}
