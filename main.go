package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/PerformLine/go-stockutil/colorutil"
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	//For adjusting chroma
	DoutDir          = "out"
	DdoLuma          = true
	DdoChroma        = true
	DcolorBrightness = 0.5
	DcolorSaturation = 0.8

	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 2500

	//Resolution of the output image
	DimgWidth  = 3840
	DimgHeight = 2160

	//This is the X,Y size, number of samples per pixel is superSample*superSample
	DsuperSample = 16 //max 255 (255*255=65kSample)

	//Stop rendering at this frame
	DendFrame = 3600

	//Area of interest
	DoffX = -0.2925598845093559
	DoffY = -0.45788116850031885

	//Pow 100 is constant speed
	DzoomPow = 100.0

	//Rendering optimize
	DescapeVal = 4.0

	//Zoom multiplier for quick animation previews
	DzoomAdd = 1

	//Gamma settings for color and luma. 0.4545... is standard 2.2
	DgammaLuma   = 1.0
	DgammaChroma = 1.0

	//Pixel x,y size for each thread
	//Smaller blocks prevent idle threads near end of image render
	//Really helps process scheduler on windows
	DworkBlock = 32

	//How much color rotates (in degrees) per iteration
	DcolorDegPerInter = 1

	//zoom speed divisor
	DzSpeedDiv = 1.1
)

var (
	doChroma         *bool
	doLuma           *bool
	outDir           *string
	imgWidth         *float64
	imgHeight        *float64
	superSample      *float64
	endFrame         *float64
	offX             *float64
	offY             *float64
	zoomPow          *float64
	escapeVal        *float64
	gammaLuma        *float64
	gammaChroma      *float64
	zoomAdd          *float64
	zSpeedDiv        *float64
	colorDegPerInter *int
	numThreads       *float64
	workBlock        *float64
	colorBrightness  *float64
	colorSaturation  *float64
	numInterations   *int

	//Sleep this long before starting a new thread
	//Doesn't affect performance that much, but helps multitasking
	threadSleep time.Duration = time.Microsecond * 100

	//Gamma LUT tables
	paletteL [(maxIters - preIters) + 1]uint32
	paletteC [0xFF]uint32

	//Image buffer
	offscreen  *image.Gray16
	offscreenC *image.RGBA

	//Current zoom level
	curZoom float64 = 1.0
	//zoom step size
	zoomDiv float64
	//Integer zoom is based on
	zoomInt float64
	//Frame count
	frameCount float64 = 0
	//Multithread group
	wg sizedwaitgroup.SizedWaitGroup
	//Divide by this to get average pixel color for supersampling
	numSamples uint32
	//number of times to iterate a sample
	numIters uint32
)

type Game struct {
}

func main() {

	DnumThreads := float64(runtime.NumCPU())

	doChroma = flag.Bool("doChroma", true, "output chroma/color image")
	doLuma = flag.Bool("doLuma", true, "output luma/brightness image")
	outDir = flag.String("outDir", DoutDir, "output directory")
	imgWidth = flag.Float64("width", DimgWidth, "Width of output image")
	imgHeight = flag.Float64("height", DimgHeight, "Height of output image")
	superSample = flag.Float64("super", DsuperSample, "Super sampling factor")
	endFrame = flag.Float64("end", DendFrame, "End frame")
	offX = flag.Float64("offx", DoffX, "X offset")
	offY = flag.Float64("offy", DoffY, "Y offset")
	zoomPow = flag.Float64("zoom", DzoomPow, "Zoom power")
	escapeVal = flag.Float64("escape", DescapeVal, "Escape value")
	gammaLuma = flag.Float64("gammaLuma", DgammaLuma, "Luma gamma")
	gammaChroma = flag.Float64("gammaChroma", DgammaChroma, "Chroma gamma")
	zoomAdd = flag.Float64("zoomAdd", DzoomAdd, "Zoom step size")
	zSpeedDiv = flag.Float64("zSpeedDiv", DzSpeedDiv, "Zoom speed divisor")
	colorDegPerInter = flag.Int("colorDegPerInter", DcolorDegPerInter, "Color rotation per iteration")
	numThreads = flag.Float64("numThreads", DnumThreads, "Number of threads")
	workBlock = flag.Float64("workBlock", DworkBlock, "Work block size (x*y)")
	colorBrightness = flag.Float64("colorBrightness", DcolorBrightness, "HSV brightness of the chroma.")
	colorSaturation = flag.Float64("colorSaturation", DcolorSaturation, "HSV saturation of the chroma.")
	numInterations = flag.Int("iters", maxIters, "number of iterations max")
	flag.Parse()

	//zoom step size
	zoomDiv = 10000.0 / *zSpeedDiv
	//Integer zoom is based on
	zoomInt = 9800 / *zSpeedDiv

	//Alloc images
	offscreen = image.NewGray16(image.Rect(0, 0, int(*imgWidth), int(*imgHeight)))
	offscreenC = image.NewRGBA(image.Rect(0, 0, int(*imgWidth), int(*imgHeight)))

	//Setup
	wg = sizedwaitgroup.New(int(*numThreads))
	numSamples = uint32(int(*superSample) * int(*superSample))
	numIters = uint32(*numInterations) - preIters

	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint32(math.Pow(float64(i)/float64(numIters), *gammaLuma) * 0xFFFF)
	}
	for i := range paletteC {
		paletteC[i] = uint32(math.Pow(float64(i)/float64(0xFF), *gammaChroma) * 0xFF)
	}

	//Zoom needs a pre-calculation
	calcZoom()

	//Render loop
	for {
		//Render frame
		rendered := updateOffscreen()

		//Update zoom for next frame
		calcZoom()

		//If we have a result, write it
		//(we can skip frames for resume and multi-machine rendering)
		if rendered {

			if *doChroma {
				fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "chroma", frameCount)
				output, err := os.Create(fileName)
				opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
				if tiff.Encode(output, offscreenC, opt) != nil {
					log.Println("ERROR: Failed to write image:", err)
					os.Exit(1)
				}
				output.Close()
			}

			if *doLuma {
				fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "luma", frameCount)
				output, err := os.Create(fileName)
				opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
				if tiff.Encode(output, offscreen, opt) != nil {
					log.Println("ERROR: Failed to write image:", err)
					os.Exit(1)
				}
				output.Close()
			}

			fmt.Println("Completed frame:", frameCount)
		}
		if frameCount >= *endFrame {
			fmt.Println("Rendering complete")
			os.Exit(0)
			return
		}
		frameCount++
	}
}

func updateOffscreen() bool {

	//Skip frames that already exist
	//Otherwise make a empty placeholder file to reserve this frame for us
	//For lazy file-share multi-machine rendering (i use sshfs)
	if *doChroma {
		fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "chroma", frameCount)
		_, err := os.Stat(fileName)
		if err == nil {
			fmt.Println(fileName, "chroma exists... Skipping")
			return false
		} else {
			_, err := os.Create(fileName)
			if err != nil {
				log.Println("ERROR: Failed to create file:", err)
				return false
			}
		}
	}
	if *doLuma {
		fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "luma", frameCount)
		_, err := os.Stat(fileName)
		if err == nil {
			fmt.Println(fileName, "luma exists... Skipping")
			return false
		} else {
			_, err := os.Create(fileName)
			if err != nil {
				log.Println("ERROR: Failed to create file:", err)
				return false
			}
		}
	}

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

				//Render the block
				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						var pixel uint32 = 0
						var r, g, b uint32
						var sx, sy float64

						//Supersample
						for sx = 0; sx < *superSample; sx++ {
							for sy = 0; sy < *superSample; sy++ {
								//Get the sub-pixel position
								ssx := float64(sx) / float64(*superSample)
								ssy := float64(sy) / float64(*superSample)

								//Translate to position on the mandelbrot
								xx := ((((float64(x) + ssx) / *imgWidth) - 0.5) / curZoom) - *offX
								yy := ((((float64(y) + ssy) / *imgWidth) - 0.3) / curZoom) - *offY

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

									tr, tg, tb := colorutil.HsvToRgb(float64(it*uint32(*colorDegPerInter)%360), *colorSaturation, *colorBrightness)
									//We already gamma corrected, so use gamma 1.0 for chroma
									//But still convert from 8 bits to 16, to match the luma
									r += paletteC[tr]
									g += paletteC[tg]
									b += paletteC[tb]
								}
							}
						}

						if *doLuma {
							offscreen.Set(int(x), int(y), color.Gray16{uint16(pixel / numSamples)})
						}
						if *doChroma {
							offscreenC.Set(int(x), int(y), color.RGBA{
								uint8((r / numSamples)),
								uint8((g / numSamples)),
								uint8((b / numSamples)), 0xFF})
						}
					}
				}
			}(xBlock, yBlock)
		}
	}
	wg.Wait()

	return true
}

func calcZoom() {
	zoomInt = zoomInt + *zoomAdd
	sStep := zoomInt / zoomDiv
	curZoom = math.Pow(sStep, *zoomPow)
}
