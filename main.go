package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	DimgWidth  = 512
	DimgHeight = 512

	fileName = "coordinates.txt"
	//Scroll wheel speed
	wheelMult = 50

	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 1500

	//Resolution of the output image
	DpixMag = 2

	mouseSpeed = 6.5

	//Pow 100 is constant speed
	DzoomPow = 100.0

	//Rendering optimize
	DescapeVal = 4.0

	//Pixel x,y size for each thread
	//Smaller blocks prevent idle threads near end of image render
	//Really helps process scheduler on windows
	DworkBlock = 32

	//Gamma settings for color and luma. 0.4545... is standard 2.2
	DgammaLuma = 0.5
)

var (
	sizeChanged bool
	wroteFile   time.Time

	imgWidth   *float64
	imgHeight  *float64
	zoomPow    *float64
	escapeVal  *float64
	gammaLuma  *float64
	numThreads *int
	workBlock  *float64
	pixMag     *float64

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

	//Multithread group
	wg sizedwaitgroup.SizedWaitGroup

	//number of times to iterate a sample
	numIters     uint32
	rightPressed bool

	renderWidth            float64
	renderHeight           float64
	lastMouseX, lastMouseY int

	camX, camY           float64
	tX, tY, diffX, diffY int

	drawScreen bool
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
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		if !rightPressed {
			rightPressed = true

			buf := fmt.Sprintf("%v:\n%v,%v\n\n", time.Now(), camX, camY)
			f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)

			if err == nil {
				_, err := f.WriteString(buf)
				if err == nil {
					wroteFile = time.Now()
				}
				f.Close()
			}
		}
	} else {
		rightPressed = false
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
	if sizeChanged {
		offscreen = ebiten.NewImage(int(*imgWidth), int(*imgHeight))
		updateOffscreen()
		sizeChanged = false
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(*pixMag, *pixMag)
	screen.DrawImage(ebiten.NewImageFromImage(offscreen), op)

	message := ""
	if time.Since(wroteFile) < time.Second*5 {
		message = fmt.Sprintf("Wrote coordinates to %v.", fileName)
		ebitenutil.DebugPrint(screen, message)
	} else {
		ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f (drag move, wheel zoom) %v,%v,%v", ebiten.CurrentFPS(), camX, camY, curZoom))
	}

	drawScreen = true
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	tw := float64(outsideWidth)
	th := float64(outsideHeight)

	if tw != *imgWidth || th != *imgHeight {
		sizeChanged = true
		imgWidth = &tw
		imgHeight = &th
	}
	return outsideWidth, outsideHeight
}

func main() {

	DnumThreads := runtime.NumCPU()

	imgWidth = flag.Float64("width", DimgWidth, "Width of output image")
	imgHeight = flag.Float64("height", DimgHeight, "Height of output image")
	gammaLuma = flag.Float64("gammaLuma", DgammaLuma, "Luma gamma")
	zoomPow = flag.Float64("zoom", DzoomPow, "Zoom power")
	escapeVal = flag.Float64("escape", DescapeVal, "Escape value")
	numThreads = flag.Int("numThreads", DnumThreads, "Number of threads")
	workBlock = flag.Float64("workBlock", DworkBlock, "Work block size (x*y)")
	pixMag = flag.Float64("pixMag", DpixMag, "Work block size (x*y)")
	flag.Parse()

	renderWidth = float64(*imgWidth)
	renderHeight = float64(*imgHeight)

	//zoom step size
	zoomDiv = 10000.0
	//Integer zoom is based on
	zoomInt = 9900

	//Setup
	wg = sizedwaitgroup.New(*numThreads)
	numIters = maxIters - preIters

	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint8(math.Pow(float64(i)/float64(numIters), *gammaLuma) * 0xFF)
	}

	//Zoom needs a pre-calculation
	calcZoom()

	ebiten.SetWindowSize(int((*imgWidth)*(*pixMag)), int((*imgHeight)*(*pixMag)))
	ebiten.SetWindowResizable(true)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

	offscreen = ebiten.NewImage(int(renderWidth), int(renderHeight))

	go func() {
		for {
			if drawScreen {
				for {
					updateOffscreen()
				}
			} else {
				time.Sleep(time.Millisecond * 100)
			}
		}
	}()

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

func updateOffscreen() {

	var xBlock, yBlock float64
	for xBlock = 0; xBlock <= *imgWidth / *workBlock; xBlock++ {
		for yBlock = 0; yBlock <= *imgHeight / *workBlock; yBlock++ {

			wg.Add()
			go func(xBlock, yBlock float64) {
				defer wg.Done()

				//Create a block of pixels for the thread to work on
				xStart := xBlock * *workBlock
				yStart := yBlock * *workBlock

				xEnd := xStart + *workBlock
				yEnd := yStart + *workBlock

				//Don't render off the screen
				if xStart < 0 {
					xStart = 0
				}
				if yStart < 0 {
					yStart = 0
				}
				if xEnd > *imgWidth {
					xEnd = *imgWidth
				}
				if yEnd > *imgHeight {
					yEnd = *imgHeight
				}

				//Render the block
				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						//Translate to position on the mandelbrot
						xx := ((float64(x)/float64(renderWidth) - 0.5) / curZoom) - camX
						yy := ((float64(y)/float64(renderWidth) - 0.5) / curZoom) - camY

						c := complex(xx, yy) //Rotate
						z := complex(0, 0)

						var it uint32 = 0
						skip := false
						found := false

						//Pre-interate (no draw)
						//Speed + asthetic choice
						for i := 0; i < preIters; i++ {
							z = z*z + c
							if real(z)*real(z)+imag(z)*imag(z) > *escapeVal {
								skip = true
								break
							}
						}

						//Don't render at all if we escaped in the pre-iteration.
						if !skip {
							for it = 0; it < numIters; it++ {
								z = z*z + c
								if real(z)*real(z)+imag(z)*imag(z) > *escapeVal {
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
							offscreen.Set(int(x), int(y), color.Gray{paletteL[it]})
						} else {
							offscreen.Set(int(x), int(y), color.Gray{0})
						}
					}
				}
			}(xBlock, yBlock)
		}
	}
	wg.Wait()
}

func calcZoom() {
	sStep := zoomInt / zoomDiv
	curZoom = math.Pow(sStep, *zoomPow)
}
