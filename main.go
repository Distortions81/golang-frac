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

	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	//For adjusting chroma
	DoutDir = "out"
	//Half, to combine luma/chroma
	DExposure = 0x8000
	//Underexpose for color reasons
	DcolorBrightness = 0.5
	//Desaturate a bit
	DcolorSaturation = 0.8

	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 800

	//Resolution of the output image
	DimgWidth  = 3840
	DimgHeight = 2160

	//This is the X,Y size, number of samples per pixel is superSample*superSample
	DsuperSample = 16 //max 255 (255*255=65kSample)

	//Stop rendering at this frame
	DendFrame = 3600

	//Area of interest
	DoffX = 0.6135090622704931
	DoffY = -0.6775767173961638

	//Pow 100 is constant speed
	DzoomPow = 100.0

	//Rendering optimize
	DescapeVal = 4.0

	//Zoom multiplier for quick animation previews
	DzoomAdd = 1

	//Gamma settings for color and luma. 0.4545... is standard 2.2
	DgammaLuma   = 0.5
	DgammaChroma = 1.0

	//Pixel x,y size for each thread
	//Smaller blocks prevent idle threads near end of image render
	//Really helps process scheduler on windows
	DworkBlock = 64

	//How much color rotates (in degrees) per iteration
	DcolorDegPerInter = 5.0

	//zoom speed divisor
	DzSpeedDiv = 1.1
)

var (
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
	colorDegPerInter *float64
	numThreads       *float64
	workBlock        *float64
	colorBrightness  *float64
	colorSaturation  *float64

	//Sleep this long before starting a new thread
	//Doesn't affect performance that much, but helps multitasking
	threadSleep time.Duration = time.Millisecond

	//Gamma LUT tables
	paletteL [(maxIters - preIters) + 1]uint32
	paletteC [DExposure + 1]uint32

	//Image buffer
	offscreen *image.RGBA64

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
	numSamples    float64
	numSamplesInt uint32
	//number of times to iterate a sample
	numIters uint32
)

type Game struct {
}

func main() {

	DnumThreads := float64(runtime.NumCPU())

	outDir = flag.String("outDir", DoutDir, "output directory name")
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
	colorDegPerInter = flag.Float64("colorDegPerInter", DcolorDegPerInter, "Color rotation per iteration")
	numThreads = flag.Float64("numThreads", DnumThreads, "Number of threads")
	workBlock = flag.Float64("workBlock", DworkBlock, "Work block size (x*y)")
	colorBrightness = flag.Float64("colorBrightness", DcolorBrightness, "HSV brightness of the chroma.")
	colorSaturation = flag.Float64("colorSaturation", DcolorSaturation, "HSV saturation of the chroma.")
	flag.Parse()

	err := os.MkdirAll(*outDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	//zoom step size
	zoomDiv = 10000.0 / *zSpeedDiv
	//Integer zoom is based on
	zoomInt = 9800 / *zSpeedDiv

	//Alloc images
	offscreen = image.NewRGBA64(image.Rect(0, 0, int(*imgWidth), int(*imgHeight)))

	//Setup
	wg = sizedwaitgroup.New(int(*numThreads))
	numSamples = float64(int(*superSample) * int(*superSample))
	numSamplesInt = uint32(numSamples)
	numIters = maxIters - preIters

	//Half, 0x8000 for combine
	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint32(math.Pow(float64(i)/float64(numIters), *gammaLuma) * DExposure)
	}
	for i := range paletteC {
		paletteC[i] = uint32(math.Pow(float64(i)/float64(DExposure), *gammaChroma) * DExposure)
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

			fileName := fmt.Sprintf("%v/color-%v.tif", *outDir, frameCount)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreen, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()

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
	fileName := fmt.Sprintf("%v/color-%v.tif", *outDir, frameCount)
	_, err := os.Stat(fileName)
	if err == nil {
		fmt.Println(fileName, "Exists... Skipping")
		return false
	} else {
		_, err := os.Create(fileName)
		if err != nil {
			log.Println("ERROR: Failed to create file:", err)
			return false
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

				//If the block is off the screen entirely, skip it
				if xStart > *imgWidth {
					return
				}
				if yStart > *imgHeight {
					return
				}

				//Render the block
				var sx, sy, ssx, ssy, cx, cy float64
				var it uint32
				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						var pixel uint32 = 0
						var r, g, b float64

						//Supersample
						for sx = 0; sx < *superSample; sx++ {
							for sy = 0; sy < *superSample; sy++ {

								skip := false
								found := false

								//Get the sub-pixel position
								ssx = float64(sx) / float64(*superSample)
								ssy = float64(sy) / float64(*superSample)
								//Translate to position on the mandelbrot
								cx = ((((float64(x) + ssx) / *imgWidth) - 0.5) / curZoom) - *offX
								cy = ((((float64(y) + ssy) / *imgWidth) - 0.3) / curZoom) - *offY

								var tempx, zx, zy, zzx, zzy float64

								//Preiteration, don't draw.
								for it = 0; it < preIters; it++ {
									zzx = zx * zx
									zzy = zy * zy
									if zzx+zzy > *escapeVal {
										skip = true
										break
									}
									tempx = zzx - zzy + cx
									zy = 2*zx*zy + cy
									zx = tempx
								}

								if !skip {
									for it = 0; it < numIters; it++ {
										//Requires a copy, but halves calculation
										zzx = zx * zx
										zzy = zy * zy
										if zzx+zzy > *escapeVal {
											found = true
											break
										}
										tempx = zzx - zzy + cx
										zy = 2*zx*zy + cy
										zx = tempx
									}
								}

								if found {
									//Don't render if we didn't escape
									//This allows background and bulb to be black
									//Add the value ( gamma correct ) to the total
									//We later divide to get the average for super-sampling
									pixel += paletteL[it]

									//replaced colorutil, but still needs improvement
									rt, gt, bt := hsv2rgbf(float64(it)*float64(*colorDegPerInter), *colorSaturation, *colorBrightness)
									r += rt
									g += gt
									b += bt
								}
							}
						}

						//Add the pixel to the buffer, divide by number of samples for super-sampling
						offscreen.Set(int(x), int(y), color.RGBA64{
							uint16(paletteC[uint32(r*DExposure)/numSamplesInt] + paletteC[pixel/numSamplesInt]),
							uint16(paletteC[uint32(g*DExposure)/numSamplesInt] + paletteC[pixel/numSamplesInt]),
							uint16(paletteC[uint32(b*DExposure)/numSamplesInt] + paletteC[pixel/numSamplesInt]), 0xFFFF})

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

func hsv2rgbf(h, sat, val float64) (r, g, b float64) {

	chroma := (1 - math.Abs((2*val)-1)) * sat
	hue := math.Mod(h, 360)
	hueSector := hue / 60

	intermediate := chroma * (1 - math.Abs(
		math.Mod(hueSector, 2)-1,
	))

	switch {
	case hueSector >= 0 && hueSector <= 1:
		r = chroma
		g = intermediate
		b = 0

	case hueSector > 1 && hueSector <= 2:
		r = intermediate
		g = chroma
		b = 0

	case hueSector > 2 && hueSector <= 3:
		r = 0
		g = chroma
		b = intermediate

	case hueSector > 3 && hueSector <= 4:
		r = 0
		g = intermediate
		b = chroma
	case hueSector > 4 && hueSector <= 5:
		r = intermediate
		g = 0
		b = chroma

	case hueSector > 5 && hueSector <= 6:
		r = chroma
		g = 0
		b = intermediate

	default:
		panic(fmt.Errorf("hue input %v yielded sector %v", hue, hueSector))
	}

	return r, g, b
}
