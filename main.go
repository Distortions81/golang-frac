package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	autoZoom    = true
	startOffset = 48
	superSample = 1
	winWidth    = 1024
	winHeight   = 1024
	maxIters    = 255
	offX        = -0.77568377
	offY        = 0.13646737
	zoomSpeed   = 10
	wheelSpeed  = 0.05
	gamma       = 0.45 // 2.2
)

var (
	renderWidth  int = winWidth * superSample
	renderHeight int = winHeight * superSample

	offscreen  *image.Gray
	palette    [maxIters + 1]uint8
	numThreads = runtime.NumCPU()
	count      uint64

	curZoom float64 = 1.0
	zoomInt int     = startOffset

	drew     bool
	rendered bool = false
)

type Game struct {
	x float64
	y float64
}

func (g *Game) Update() error {
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Reset()
	op.GeoM.Scale(1.0/superSample, 1.0/superSample)
	op.Filter = ebiten.FilterLinear

	screen.DrawImage(ebiten.NewImageFromImage(offscreen), nil)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, as: %d, z: %0.2f, w: %0.2f, t: %d", ebiten.CurrentFPS(), ebiten.CurrentTPS(), zoomInt-startOffset, curZoom, g.y, numThreads))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return winWidth, winHeight
}

func main() {
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)
	ebiten.SetMaxTPS(60)

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func lut(it uint8) (c uint8) {
	if it >= maxIters {
		return 0xff
	}

	return palette[it]
}

func updateOffscreen() {
	count++

	swg := sizedwaitgroup.New(numThreads)
	for j := 0; j < renderHeight; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < renderWidth; i++ {
				x := (float64(j)/float64(renderHeight)-0.5)/curZoom*3.0 + offX
				y := (float64(i)/float64(renderHeight)-0.5)/curZoom*3.0 + offY
				c := complex(x, y) //Rotate
				z := complex(0, 0)
				var it uint8
				for ; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > 4 {
						break
					}
				}
				offscreen.Set(j, i, color.Gray{palette[it]})
			}

		}(j)
	}
	swg.Wait()

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := float64(zoomInt) / 1000.0
		curZoom = curZoom + (sStep * sStep * float64(zoomSpeed))
	}
}

func init() {

	rand.Seed(time.Now().UnixNano())
	buf := fmt.Sprintf("Threads found: %v", numThreads)
	fmt.Println(buf)
	if numThreads < 4 {
		numThreads = 2
	}

	fmt.Println("Allocating image...")

	/* Max image size */
	if renderWidth > 32768 {
		renderWidth = 32768
		fmt.Println("renderWidth > 32768, truncating...")
	}
	if renderHeight > 32768 {
		renderHeight = 32768
		fmt.Println("renderHeight > 32768, truncating...")
	}

	offscreen = image.NewGray(image.Rect(0, 0, renderWidth, renderHeight))

	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New((numThreads))
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()

			palette[i] = uint8(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xff))
		}(i)
	}

	swg.Wait()
	fmt.Printf("complete!\n")

	go func() {
		for {
			updateOffscreen()
		}
	}()
}
