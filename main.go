package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"

	"github.com/PerformLine/go-stockutil/colorutil"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	chromaMode    = true
	lumaMode      = true
	autoZoom      = true
	startOffset   = 970
	superSample   = 4
	windowDivisor = 4.0
	winWidth      = 3840
	winHeight     = 2160
	maxIters      = 360
	offX          = 0.747926709975882
	offY          = -0.10785035275635992
	zoomPow       = 100
	zoomDiv       = 1000.0
	escapeVal     = 4.0
	colorRots     = 10

	gamma = 0.4545
)

var (
	renderWidth  int = winWidth * superSample
	renderHeight int = winHeight * superSample

	offscreen     *image.RGBA
	offscreenGray *image.Gray16

	downresChroma *ebiten.Image
	downresLuma   *ebiten.Image
	numThreads    = runtime.NumCPU()

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

	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Translate(0, 0)
	op.GeoM.Scale(1.0/windowDivisor, 1.0/windowDivisor)
	screen.DrawImage(downresChroma, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, x: %v, y: %v z: %v", ebiten.CurrentFPS(), ebiten.CurrentTPS(), camX, camY, zoomInt))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	ebiten.SetWindowSize(winWidth/windowDivisor, winHeight/windowDivisor)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)
	ebiten.SetMaxTPS(30)

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
				var it uint16
				for it = 0; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
						break
					}
				}
				if chromaMode {
					r, g, b := colorutil.HsvToRgb(float64(it)/float64(maxIters)*360.0, 1.0, 1.0)
					offscreen.Set(j, i, color.RGBA{r, g, b, 255})
				}
				if lumaMode {
					offscreenGray.Set(j, i, color.Gray16{it})
				}
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
	if autoZoom {
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(1.0/superSample, 1.0/superSample)
		if chromaMode {
			downresChroma.DrawImage(ebiten.NewImageFromImage(offscreen), op)

			fileName := fmt.Sprintf("out/color-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, downresChroma, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}
		if lumaMode {
			downresLuma.DrawImage(ebiten.NewImageFromImage(offscreenGray), op)

			fileName := fmt.Sprintf("out/luma-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, downresLuma, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}
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

	offscreen = image.NewRGBA(image.Rect(0, 0, renderWidth, renderHeight))
	offscreenGray = image.NewGray16(image.Rect(0, 0, renderWidth, renderHeight))
	downresChroma = ebiten.NewImage(renderWidth/superSample, renderHeight/superSample)
	downresLuma = ebiten.NewImage(renderWidth/superSample, renderHeight/superSample)

	fmt.Printf("complete!\n")

	go func() {
		for {
			updateOffscreen()
		}
	}()
}
