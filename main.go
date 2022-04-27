package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	autoZoom    = true
	startOffset = 48
	superSample = 4
	winWidth    = 1024
	winHeight   = 1024
	maxIters    = 1024
	offX        = -0.77568377
	offY        = 0.13646737
	zoomSpeed   = 10
	gamma       = 0.4
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

	screen.DrawImage(ebiten.NewImageFromImage(offscreen), op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, z: %0.2f", ebiten.CurrentFPS(), ebiten.CurrentTPS(), curZoom))
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
				var it uint16
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
	buf := fmt.Sprintf("Threads found: %v", numThreads)
	fmt.Println(buf)
	if numThreads < 1 {
		numThreads = 1
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
