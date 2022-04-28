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
	chromaMode       = true
	lumaMode         = true
	autoZoom         = true
	startOffset      = 9800
	superSample      = 8
	windowDivisor    = 4.0
	winWidth         = 3840
	winHeight        = 2160
	maxIters         = 10000
	offX             = 0.747926709975882
	offY             = -0.10785035275635992
	zoomPow          = 100
	zoomDiv          = 10000.0
	escapeVal        = 4.0
	colorDegPerInter = 10

	gamma = 0.4545
)

var (
	palette      [maxIters + 1]uint16
	renderWidth  int = winWidth * superSample
	renderHeight int = winHeight * superSample

	minBright uint16 = 0xffff
	maxBright uint16 = 0x0000

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

	if !autoZoom {
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
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {

	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Reset()
	op.GeoM.Translate(0, 0)
	op.GeoM.Scale(1/windowDivisor, 1/windowDivisor)
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

	if offscreen == nil || offscreenGray == nil || downresChroma == nil || downresLuma == nil {
		fmt.Println("Failed to allocate image")
		return
	}
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

	sStep := (float64(zoomInt) / zoomDiv)
	curZoom = (math.Pow(sStep, zoomPow))

	go func() {
		for {
			updateOffscreen()
		}
	}()

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func updateOffscreen() {

	swg := sizedwaitgroup.New(numThreads)
	maxBright = 0x0000
	minBright = 0xffff
	for j := 0; j < renderWidth; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < renderHeight; i++ {

				x := ((float64(j)/float64(renderWidth) - 0.5) / curZoom) - camX
				y := ((float64(i)/float64(renderWidth) - 0.25) / curZoom) - camY
				c := complex(x, y) //Rotate
				z := complex(0, 0)

				var it uint16
				for it = 0; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
						break
					}
				}

				r, g, b := colorutil.HsvToRgb(math.Mod(float64(it*colorDegPerInter), 360), 1.0, 1.0)
				offscreen.Set(j, i, color.RGBA{r, g, b, 255})

				if lumaMode {
					offscreenGray.Set(j, i, color.Gray16{palette[it]})
					if it > maxBright {
						maxBright = it
					}
					if it < minBright {
						minBright = it
					}
				}
			}

		}(j)
	}
	swg.Wait()

	//Used for display, replace pixes is faster but we need the resizing and filtering here
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(1.0/superSample, 1.0/superSample)
	downresChroma.DrawImage(ebiten.NewImageFromImage(offscreen), op)

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))

		if chromaMode {

			fileName := fmt.Sprintf("out/color-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, downresChroma, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}

		/*Auto contrast*/
		if lumaMode {
			//Auto constrast limits
			if minBright > 51000 {
				minBright = 51000
			}
			if maxBright < 51255 {
				maxBright = 51255
			}

			for j := 0; j < renderWidth; j++ {
				swg.Add()
				go func(j int) {
					defer swg.Done()

					for i := 0; i < renderHeight; i++ {
						pixel := offscreenGray.Gray16At(j, i)
						y := pixel.Y
						dim := y - minBright //Subtract so black is black
						if dim > 0 {
							//Increase constast
							out := uint16(float64(dim) / (float64(65535-minBright-(65535-maxBright)) / 65535.0))
							offscreenGray.Set(j, i, color.Gray16{out})
						} else {
							offscreenGray.Set(j, i, color.Gray16{0})
						}
					}

				}(j)
			}

			swg.Wait()

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

	//
}
