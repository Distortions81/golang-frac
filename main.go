package main

import (
	"fmt"
	"image/color"
	"log"
	"math"

	"github.com/PerformLine/go-stockutil/colorutil"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	startOffset = 985
	winWidth    = 1024
	winHeight   = 1024
	pixMag      = 2
	maxIters    = 255
	zoomPow     = 100
	zoomDiv     = 1000.0
	escapeVal   = 4.0
	camZoomDiv  = 1
	wheelMult   = 4

	colorSat         = 0.8
	colorBright      = 0.75
	colorDegPerInter = 10
)

var (
	renderWidth  int = winWidth / pixMag
	renderHeight int = winHeight / pixMag

	offscreen *ebiten.Image

	curZoom                float64 = 1.0
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
	updateOffscreen()

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(pixMag, pixMag)
	screen.DrawImage(offscreen, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f (click drag to move, wheel to zoom)", ebiten.CurrentFPS()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

	offscreen = ebiten.NewImage(renderWidth, renderHeight)

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

var r, g, b uint8
var x, y float64
var c, z complex128
var j, i, it int

func updateOffscreen() {

	for j = 0; j < renderWidth; j++ {

		for i = 0; i < renderHeight; i++ {
			x = ((float64(j)/float64(renderWidth) - 0.5) / curZoom) - camX
			y = ((float64(i)/float64(renderWidth) - 0.5) / curZoom) - camY
			c = complex(x, y) //Rotate
			z = complex(0, 0)

			for it = 0; it < maxIters; it++ {
				z = z*z + c
				if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
					break
				}
			}
			r, g, b = colorutil.HsvToRgb(math.Mod(float64(it*colorDegPerInter), 360), colorSat, colorBright)
			offscreen.Set(j, i, color.RGBA{r, g, b, 0xff})
		}

	}
}

func init() {
	//
}
