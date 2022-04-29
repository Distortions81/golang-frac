package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/PerformLine/go-stockutil/colorutil"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	headlessMode     = true
	chromaMode       = true
	lumaMode         = true
	autoZoom         = true
	startOffset      = 9900
	winWidth         = 3840
	winHeight        = 2160
	superSamples     = 16 //max 16x16
	maxIters         = 0xFFFF
	offX             = 0.747926709975882
	offY             = -0.10785035275635992
	zoomPow          = 100
	zoomDiv          = 10000.0
	escapeVal        = 4.0
	colorDegPerInter = 10
	flopDrawSeconds  = 10
	doJitter         = true
	fastJitter       = true

	jitterDiv = superSamples //rand is 0 to 1, divide by this to get jitter

	gamma = 0.8
)

var (
	palette      [maxIters + 1]uint16
	renderWidth  int    = winWidth
	renderHeight int    = winHeight
	minBright    uint16 = 0xffff
	maxBright    uint16 = 0x0000

	screenBuffer  *ebiten.Image
	offscreen     *image.RGBA64
	offscreenGray *image.Gray16

	numThreads = runtime.NumCPU()

	curZoom                float64 = 1.0
	zoomInt                int     = startOffset
	frameNum               uint64  = 0
	drawNum                uint64  = 0
	lastMouseX, lastMouseY int
	flopDraw               bool
	lastDraw               time.Time

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
	drawNum++
	screen.Clear()
	if time.Since(lastDraw).Seconds() > flopDrawSeconds {
		lastDraw = time.Now()
		if flopDraw {
			flopDraw = false
		} else {
			flopDraw = true
		}
	}
	if flopDraw {
		screenBuffer = ebiten.NewImageFromImage(offscreen)
	} else {
		screenBuffer = ebiten.NewImageFromImage(offscreenGray)
	}
	screen.DrawImage(screenBuffer, nil)
	screenBuffer.Dispose()
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f, UPS: %0.2f, x: %v, y: %v z: %v", ebiten.CurrentFPS(), ebiten.CurrentTPS(), camX, camY, zoomInt))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	ebiten.SetWindowSize(winWidth, winHeight)
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

	offscreen = image.NewRGBA64(image.Rect(0, 0, renderWidth, renderHeight))
	offscreenGray = image.NewGray16(image.Rect(0, 0, renderWidth, renderHeight))

	if offscreen == nil || offscreenGray == nil {
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

	if !headlessMode {
		if err := ebiten.RunGame(&Game{}); err != nil {
			log.Fatal(err)
		}
	} else {
		for {
			time.Sleep(time.Second)
		}
	}
}

func updateOffscreen() {
	frameStart := true
	swg := sizedwaitgroup.New(numThreads)
	maxBright = 0x0000
	minBright = 0xffff
	var x, y, xjit, yjit float64

	for sx := 1; sx < superSamples; sx++ {
		for sy := 1; sy <= superSamples; sy++ {
			if doJitter && fastJitter {
				xjit = rand.Float64() / jitterDiv
				yjit = rand.Float64() / jitterDiv
			}
			rand.Seed(time.Now().UnixNano())
			for j := 0; j < renderWidth; j++ {
				swg.Add()
				go func(j int) {
					defer swg.Done()
					for i := 0; i < renderHeight; i++ {
						ssx := (float64(sx) / float64(superSamples))
						ssy := (float64(sy) / float64(superSamples))

						if doJitter && !fastJitter {
							xjit = rand.Float64() / jitterDiv
							yjit = rand.Float64() / jitterDiv
						}
						x = (((float64(j)+ssx+xjit)/float64(renderWidth) - 0.5) / (curZoom)) - camX
						y = (((float64(i)+ssy+yjit)/float64(renderWidth) - 0.3) / (curZoom)) - camY

						c := complex(x, y) //Rotate
						z := complex(0, 0)

						var it uint16
						for it = 0; it < maxIters; it++ {
							z = z*z + c
							if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
								break
							}
						}

						if frameStart {
							r, g, b := colorutil.HsvToRgb(math.Mod(float64(it*colorDegPerInter), 360), 1.0, 1.0)
							offscreen.Set(j, i, color.RGBA64{uint16(r), uint16(g), uint16(b), 0xFFFF})
						} else {
							r, g, b := colorutil.HsvToRgb(math.Mod(float64(it*colorDegPerInter), 360), 1.0, 1.0)
							or, og, ob, _ := offscreen.At(j, i).RGBA()
							if r > 254 {
								r = 255
							}
							if g > 254 {
								g = 255
							}
							if b > 254 {
								b = 255
							}
							offscreen.Set(j, i, color.RGBA64{uint16(uint32(r) + or), uint16(uint32(g) + og), uint16(uint32(b) + ob), 0xFFFF})
						}

						if lumaMode {
							if frameStart {
								if it > 255 {
									it = 255
								}
								offscreenGray.Set(j, i, color.Gray16{it})

								if it > maxBright {
									maxBright = it
								}
								if it < minBright {
									minBright = it
								}
							} else {
								y := offscreenGray.At(j, i).(color.Gray16).Y
								if it > 255 {
									it = 255
								}

								offscreenGray.Set(j, i, color.Gray16{(it + y)})
							}
						}
					}

				}(j)
			}
			frameStart = false
			fmt.Println("Pass:", sx+sy-1, "/", superSamples*superSamples, "Frame:", frameNum)
		}
	}

	swg.Wait()

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))

		if chromaMode {

			fileName := fmt.Sprintf("out/color-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreen, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}

		if lumaMode {
			fileName := fmt.Sprintf("out/luma-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreenGray, opt) != nil {
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
