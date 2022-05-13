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
	//Scroll wheel speed
	wheelMult = 50

	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 700

	//Resolution of the output image
	DimgWidth  = 512
	DimgHeight = 512
	DpixMag    = 3

	mouseSpeed = 6.5

	//Pow 100 is constant speed
	DzoomPow = 100.0

	//Rendering optimize
	DescapeVal = 4.0

	//Pixel x,y size for each thread
	//Smaller blocks prevent idle threads near end of image render
	//Really helps process scheduler on windows
	DworkBlock = 64

	//Gamma settings for color and luma. 0.4545... is standard 2.2
	DgammaLuma = 0.5
)

var (
	//Gamma LUT tables
	paletteL [(maxIters - preIters) + 1]uint8

	//Image buffer
	offscreen *ebiten.Image

	//Current zoom level
	curZoom float64
	//zoom step size
	zoomDiv float64
	//Integer zoom is based on
	zoomInt float64

	//number of times to iterate a sample
	numIters uint32

	renderWidth            float64
	renderHeight           float64
	lastMouseX, lastMouseY int

	camX, camY           float64
	tX, tY, diffX, diffY int
)

type Game struct {
}

func (g *Game) Update() error {

	tX, tY = ebiten.CursorPosition()
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		diffX = (tX - lastMouseX)
		diffY = (tY - lastMouseY)

		camX += ((float64(diffX) * mouseSpeed) / (float64(zoomInt) * curZoom))
		camY += ((float64(diffY) * mouseSpeed) / (float64(zoomInt) * curZoom))
	}

	lastMouseX = tX
	lastMouseY = tY

	_, fsy := ebiten.Wheel()
	if fsy > 0 {
		zoomInt += wheelMult
	} else if fsy < 0 {
		zoomInt -= wheelMult
	}

	calcZoom()
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	updateOffscreen()

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(DpixMag, DpixMag)
	screen.DrawImage(offscreen, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f (click drag to move, wheel to zoom) %v,%v", ebiten.CurrentFPS(), camX, camY))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	renderWidth = DimgWidth
	renderHeight = DimgHeight

	//zoom step size
	zoomDiv = 10000.0
	//Integer zoom is based on
	zoomInt = 9900

	//Setup
	numIters = maxIters - preIters

	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint8(math.Pow(float64(i)/float64(numIters), DgammaLuma) * 0xFF)
	}

	//Zoom needs a pre-calculation
	calcZoom()

	ebiten.SetWindowSize(int((DimgWidth)*(DpixMag)), int((DimgHeight)*(DpixMag)))
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

	offscreen = ebiten.NewImage(int(renderWidth), int(renderHeight))

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func updateOffscreen() {

	var x, y float64
	for x = 0; x <= DimgWidth; x++ {
		for y = 0; y <= DimgHeight; y++ {

			var pixel uint8 = 0

			//Translate to position on the mandelbrot
			xx := ((((float64(x)) / DimgWidth) - 0.5) / curZoom) - camX
			yy := ((((float64(y)) / DimgWidth) - 0.5) / curZoom) - camY

			c := complex(xx, yy) //Rotate
			z := complex(0, 0)

			var it uint32 = 0
			skip := false
			found := false

			//Pre-interate (no draw)
			//Speed + asthetic choice
			for i := 0; i < preIters; i++ {
				z = z*z + c
				if real(z)*real(z)+imag(z)*imag(z) > DescapeVal {
					skip = true
					break
				}
			}

			//Don't render at all if we escaped in the pre-iteration.
			if !skip {
				for it = 0; it < numIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > DescapeVal {
						found = true
						break
					}
				}
			}

			//Don't render if we didn't escape
			//This allows background and bulb to be black
			if found {
				//Add the value ( gamma correct ) to the total
				//We later divide to get the average for super-sampling
				pixel += paletteL[it]
			}

			//Add the pixel to the buffer, divide by number of samples for super-sampling
			offscreen.Set(int(x), int(y), color.Gray{pixel})

		}
	}
}

func calcZoom() {
	sStep := zoomInt / zoomDiv
	curZoom = math.Pow(sStep, DzoomPow)
}
