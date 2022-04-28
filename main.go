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
	autoZoom    = false
	startOffset = 970
	winWidth    = 1280
	winHeight   = 720
	maxIters    = 255
	offX        = 0
	offY        = 0
	zoomPow     = 100
	zoomDiv     = 1000.0
	escapeVal   = 4.0

	gamma = 0.6
)

var (
	palette      [maxIters + 1]uint8
	renderWidth  int = winWidth
	renderHeight int = winHeight

	offscreen  *image.Gray
	numThreads = runtime.NumCPU()

	curZoom                float64 = 1.0
	zoomInt                int     = startOffset
	frameNum               uint64  = 0
	lastMouseX, lastMouseY int

	camX, camY float64
	camZoomDiv float64 = 1
	wheelMult  int     = 4
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

	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Translate(0, 0)
	op.GeoM.Scale(1.0, 1.0)
	screen.DrawImage(ebiten.NewImageFromImage(offscreen), op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, x: %v, y: %v z: %v", ebiten.CurrentFPS(), ebiten.CurrentTPS(), camX, camY, zoomInt))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

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

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func updateOffscreen() {

	swg := sizedwaitgroup.New(numThreads)
	for j := 0; j < renderWidth; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < renderHeight; i++ {
				x := ((float64(j)/float64(renderWidth) - 0.5) / curZoom) - camX
				y := ((float64(i)/float64(renderWidth) - 0.3) / curZoom) - camY
				c := complex(x, y) //Rotate
				z := complex(0, 0)
				var it uint8
				for it = 0; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
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
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))
	}
	frameNum++

}

func init() {

	camX = offX
	camY = offY

	buf := fmt.Sprintf("Threads found: %v", numThreads)
	fmt.Println(buf)
	if numThreads < 1 {
		numThreads = 1
	}

	go func() {
		for {
			updateOffscreen()
		}
	}()
}
