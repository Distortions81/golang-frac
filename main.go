package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	autoZoom    = true
	startOffset = 970
	superSample = 1
	winWidth    = 512
	winHeight   = 512
	maxIters    = 1024
	offX        = -0.3663629834227643
	offY        = -0.5915337732614452
	zoomPow     = 100
	zoomDiv     = 1000.0
	escapeVal   = 4.0

	gamma = 0.4545
)

var (
	renderWidth  int = winWidth * superSample
	renderHeight int = winHeight * superSample

	offscreen  *image.Gray16
	palette    [maxIters + 1]uint16
	numThreads = runtime.NumCPU()

	curZoom                float64 = 1.0
	zoomInt                int     = startOffset
	frameNum               uint64  = 0
	lastMouseX, lastMouseY int

	camX, camY float64
	camZoomDiv float64 = 1
	wheelMult  int     = 1
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

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Reset()
	op.GeoM.Scale(1.0/superSample, 1.0/superSample)
	op.Filter = ebiten.FilterLinear

	screen.DrawImage(ebiten.NewImageFromImage(offscreen), op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, x: %v, y: %v z: %v", ebiten.CurrentFPS(), ebiten.CurrentTPS(), camX, camY, zoomInt))
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

	swg := sizedwaitgroup.New(numThreads)
	for j := 0; j < renderHeight; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < renderWidth; i++ {
				x := ((float64(j)/float64(renderHeight) - 0.5) / curZoom) - camX
				y := ((float64(i)/float64(renderHeight) - 0.5) / curZoom) - camY
				c := complex(x, y) //Rotate
				z := complex(0, 0)
				var it uint16
				for ; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
						break
					}
				}
				offscreen.Set(j, i, color.Gray16{palette[it]})
			}

		}(j)
	}
	swg.Wait()

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))
	}

	//Write the png file
	fileName := fmt.Sprintf("out/%v.tif", zoomInt)
	output, err := os.Create(fileName)
	opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
	if tiff.Encode(output, offscreen, opt) != nil {
		log.Println("ERROR: Failed to write image:", err)
		os.Exit(1)
	}
	output.Close()

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

	offscreen = image.NewGray16(image.Rect(0, 0, renderWidth, renderHeight))

	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New((numThreads))
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()

			palette[i] = uint16(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xffff))
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
