package main

import (
	"fmt"
	"image/color"
	"log"
	"math"

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
	gamma       = 0.6
)

var (
	palette      [maxIters + 1]uint8
	renderWidth  int = winWidth / pixMag
	renderHeight int = winHeight / pixMag

	offscreen *ebiten.Image

	curZoom                float64 = 1.0
	zoomInt                int     = startOffset
	lastMouseX, lastMouseY int

	camX, camY float64

	minBright uint8 = 0xff
	maxBright uint8 = 0x00
)

type Game struct {
}

func (g *Game) Update() error {

	tX, tY := ebiten.CursorPosition()
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		diffX := (tX - lastMouseX)
		diffY := (tY - lastMouseY)

		camX += ((float64(diffX) / camZoomDiv) / (float64(zoomInt) * curZoom))
		camY += ((float64(diffY) / camZoomDiv) / (float64(zoomInt) * curZoom))
	}

	lastMouseX = tX
	lastMouseY = tY

	_, fsy := ebiten.Wheel()
	if fsy > 0 {
		zoomInt += wheelMult
	} else if fsy < 0 {
		zoomInt -= wheelMult
	}

	sStep := float64(zoomInt) / zoomDiv
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

	for i := range palette {
		palette[i] = uint8(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xff))
	}

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func updateOffscreen() {
	minBright = 0xff
	maxBright = 0x00

	for j := 0; j < renderWidth; j++ {

		for i := 0; i < renderHeight; i++ {
			x := ((float64(j)/float64(renderWidth) - 0.5) / curZoom) - camX
			y := ((float64(i)/float64(renderWidth) - 0.5) / curZoom) - camY
			c := complex(x, y) //Rotate
			z := complex(0, 0)
			var it uint8
			for it = 0; it < maxIters; it++ {
				z = z*z + c
				if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
					break
				}
			}
			if palette[it] < minBright {
				minBright = palette[it]
			}
			if palette[it] > maxBright {
				maxBright = palette[it]
			}
			offscreen.Set(j, i, color.RGBA{palette[it], palette[it], palette[it], 0xff})
		}

	}

	if minBright > 200 {
		minBright = 200
	}
	if maxBright < 201 {
		maxBright = 201
	}

	/*Auto contrast*/
	if minBright < 255 && minBright > 0 {
		for j := 0; j < renderWidth; j++ {
			for i := 0; i < renderHeight; i++ {
				pixel := offscreen.At(j, i)
				r, _, _, _ := pixel.RGBA()
				v := uint8(r >> 8)   //Bitshift
				dim := v - minBright //Subtract so black is black
				if dim > 0 {
					//Increase constast
					out := uint8(float64(dim) / (float64(255-minBright-(255-maxBright)) / 255.0))
					offscreen.Set(j, i, color.RGBA{out, out, out, 0xff})
				} else {
					offscreen.Set(j, i, color.RGBA{0, 0, 0, 0xff})
				}
			}
		}
	}
}

func init() {
	//
}
