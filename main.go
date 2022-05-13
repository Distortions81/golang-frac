package main

import (
	"flag"
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
	//Scroll wheel speed
	wheelMult = 50

	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 800

	//Resolution of the output image
	DimgWidth  = 512
	DimgHeight = 512
	DpixMag    = 3

	mouseSpeed = 6.5

	//This is the X,Y size, number of samples per pixel is superSample*superSample
	DsuperSample = 1 //max 255 (255*255=65kSample)

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
	imgWidth    *float64
	imgHeight   *float64
	superSample *float64
	zoomPow     *float64
	escapeVal   *float64
	gammaLuma   *float64
	numThreads  *int
	workBlock   *float64
	pixMag      *float64

	//Sleep this long before starting a new thread
	//Doesn't affect performance that much, but helps multitasking
	threadSleep time.Duration = time.Microsecond * 100

	//Gamma LUT tables
	paletteL [(maxIters - preIters) + 1]uint32

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
	//Divide by this to get average pixel color for supersampling
	numSamples uint32
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
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(*pixMag, *pixMag)
	screen.DrawImage(ebiten.NewImageFromImage(offscreen), op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f (click drag to move, wheel to zoom) %v,%v", ebiten.CurrentFPS(), camX, camY))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {

	DnumThreads := runtime.NumCPU()

	imgWidth = flag.Float64("width", DimgWidth, "Width of output image")
	imgHeight = flag.Float64("height", DimgHeight, "Height of output image")
	superSample = flag.Float64("super", DsuperSample, "Super sampling factor")
	gammaLuma = flag.Float64("gammaLuma", DgammaLuma, "Luma gamma")
	zoomPow = flag.Float64("zoom", DzoomPow, "Zoom power")
	escapeVal = flag.Float64("escape", DescapeVal, "Escape value")
	numThreads = flag.Int("numThreads", DnumThreads, "Number of threads")
	workBlock = flag.Float64("workBlock", DworkBlock, "Work block size (x*y)")
	pixMag = flag.Float64("pixMag", DpixMag, "Work block size (x*y)")
	flag.Parse()

	renderWidth = *imgWidth
	renderHeight = *imgHeight

	//zoom step size
	zoomDiv = 10000.0
	//Integer zoom is based on
	zoomInt = 9900

	//Setup
	wg = sizedwaitgroup.New(*numThreads)
	numSamples = uint32(int(*superSample) * int(*superSample))
	numIters = maxIters - preIters

	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint32(math.Pow(float64(i)/float64(numIters), *gammaLuma) * 0xFFFF)
	}

	//Zoom needs a pre-calculation
	calcZoom()

	ebiten.SetWindowSize(int((*imgWidth)*(*pixMag)), int((*imgHeight)*(*pixMag)))
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)

	offscreen = ebiten.NewImage(int(renderWidth), int(renderHeight))

	go func() {
		time.Sleep(time.Second * 1)
		for {
			updateOffscreen()
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
			time.Sleep(threadSleep) //Give process manager a moment
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

				//If the block is off the screen entirely, skip it
				if xStart > *imgWidth {
					return
				}
				if yStart > *imgHeight {
					return
				}

				//Render the block
				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						var pixel uint32 = 0
						var sx, sy float64

						//Supersample
						for sx = 0; sx < *superSample; sx++ {
							for sy = 0; sy < *superSample; sy++ {
								//Get the sub-pixel position
								ssx := float64(sx) / float64(*superSample)
								ssy := float64(sy) / float64(*superSample)

								//Translate to position on the mandelbrot
								xx := ((((float64(x) + ssx) / *imgWidth) - 0.5) / curZoom) - camX
								yy := ((((float64(y) + ssy) / *imgWidth) - 0.5) / curZoom) - camY

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
									pixel += paletteL[it]
								}
							}
						}

						//Add the pixel to the buffer, divide by number of samples for super-sampling
						offscreen.Set(int(x), int(y), color.RGBA64{
							uint16((pixel / numSamples)),
							uint16((pixel / numSamples)),
							uint16((pixel / numSamples)), 0xFFFF})
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
