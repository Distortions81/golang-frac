package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	startOffset = 985
	winWidth    = 1024
	winHeight   = 1024
	pixMag      = 2
	maxIters    = 1000
	zoomPow     = 100
	zoomDiv     = 1000.0
	escapeVal   = 4.0
	camZoomDiv  = 1
	wheelMult   = 6
)

var (
	renderWidth  int = winWidth / pixMag
	renderHeight int = winHeight / pixMag

	offscreen *ebiten.Image

	curZoom                float64 = 1.0
	gamma                          = 0.8
	zoomInt                int     = startOffset
	lastMouseX, lastMouseY int

	camX, camY   float64
	fsy, sStep   float64
	tX, tY       int
	diffX, diffY int
)

type Game struct {
}

func (g *Game) Update() error {

	tX, tY = ebiten.CursorPosition()
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		diffX = (tX - lastMouseX)
		diffY = (tY - lastMouseY)

		camX += ((float64(diffX) / camZoomDiv) / (float64(zoomInt) * curZoom))
		camY += ((float64(diffY) / camZoomDiv) / (float64(zoomInt) * curZoom))
	}

	lastMouseX = tX
	lastMouseY = tY

	_, fsy = ebiten.Wheel()
	if fsy > 0 {
		zoomInt += wheelMult
	} else if fsy < 0 {
		zoomInt -= wheelMult
	}

	sStep = float64(zoomInt) / zoomDiv
	curZoom = (math.Pow(sStep, zoomPow))
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	//updateOffscreen()

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(pixMag, pixMag)
	screen.DrawImage(offscreen, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f (click drag to move, wheel to zoom) %f,%f", ebiten.CurrentFPS(), camX, camY))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

var wg sizedwaitgroup.SizedWaitGroup
var palette [(maxIters) + 1]uint8

func main() {
	for i := range palette {
		palette[i] = uint8(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xFF))
	}
	wg = sizedwaitgroup.New(runtime.NumCPU())

	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

	offscreen = ebiten.NewImage(renderWidth, renderHeight)

	go func() {
		for {
			updateOffscreen()
			time.Sleep(time.Millisecond)
		}
	}()

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

var r, g, b uint8
var x, y float64
var c, z complex128
var j, i, it int

func updateOffscreen() {

	//offscreen.Clear()
	for j = 0; j < renderWidth; j++ {

		wg.Add()
		go func(j int) {
			defer wg.Done()

			for i = 0; i < renderHeight; i++ {
				x = ((float64(j)/float64(renderWidth) - 0.5) / curZoom) - camX
				y = ((float64(i)/float64(renderWidth) - 0.5) / curZoom) - camY
				c = complex(x, y) //Rotate
				z = complex(0, 0)

				for it = 0; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
						offscreen.Set(j, i, color.RGBA{palette[it], palette[it], palette[it], 0xFF})
						break
					}
					offscreen.Set(j, i, color.RGBA{0, 0, 0, 0xFF})
				}
				//offscreen.Set(j, i, color.RGBA{palette[it], palette[it], palette[it], 0xFF})
			}
		}(j)
		wg.Wait()
	}
}

func init() {
}
