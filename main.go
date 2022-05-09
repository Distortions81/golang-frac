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
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	//Pre-iteraton removes the large circle around the mandelbrot
	//I think this looks nicer, and it is a bit quicker
	preIters = 10
	//Even at max zoom (quantized around 10^15 zoom), this seems to be enough
	maxIters = 800
)

var (
	//Resolution of the output image
	imgWidth  float64 = 3840
	imgHeight float64 = 2160

	//This is the X,Y size, number of samples per pixel is superSample*superSample
	superSample float64 = 16 //max 255 (255*255=65kSample)

	//Stop rendering at this frame
	endFrame float64 = 3600

	//Area of interest
	offX float64 = 0.6135090622704931
	offY float64 = -0.6775767173961638

	//Pow 100 is constant speed
	zoomPow float64 = 100.0

	//Rendering optimize
	escapeVal float64 = 4.0

	//Zoom multiplier for quick animation previews
	zoomAdd float64 = 50

	//Gamma settings for color and luma. 0.4545... is standard 2.2
	gammaLuma   float64 = 0.4545454545454545
	gammaChroma float64 = 1.0

	//Pixel x,y size for each thread
	//Smaller blocks prevent idle threads near end of image render
	//Really helps process scheduler on windows
	workBlock float64 = 64

	//How much color rotates (in degrees) per iteration
	colorDegPerInter uint32 = 15.0

	//Gamma LUT tables
	paletteL [(maxIters - preIters) + 1]uint32
	paletteC [0xFF + 1]uint32

	//Image buffer
	offscreen *image.RGBA64

	//Detect number of threads
	numThreads = runtime.NumCPU()

	//zoom speed divisor
	zspeepdiv float64 = 0.6

	//Current zoom level
	curZoom float64 = 1.0

	//zoom step size
	zoomDiv float64 = 10000.0 / zspeepdiv

	//Integer zoom is based on
	zoomInt float64 = 9800/zspeepdiv + 1000

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
	//Alloc images
	offscreen = image.NewRGBA64(image.Rect(0, 0, int(imgWidth), int(imgHeight)))

	//Setup
	wg = sizedwaitgroup.New(numThreads)
	numSamples = uint32(superSample * superSample)
	numIters = maxIters - preIters

	//Make gamma LUTs
	for i := range paletteL {
		paletteL[i] = uint32(math.Pow(float64(i)/float64(numIters), gammaLuma) * 0xFFFF)
	}
	for i := range paletteC {
		paletteC[i] = uint32(math.Pow(float64(i)/float64(0xFF), gammaChroma) * 0xFFFF)
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

			fileName := fmt.Sprintf("out/color-%v.tif", frameCount)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreen, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()

			fmt.Println("Completed frame:", frameCount)
		}
		if frameCount >= endFrame {
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
	fileName := fmt.Sprintf("out/color-%v.tif", frameCount)
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
	for xBlock = 0; xBlock <= imgWidth/workBlock; xBlock++ {
		for yBlock = 0; yBlock <= imgHeight/workBlock; yBlock++ {

			wg.Add()
			go func(xBlock, yBlock float64) {
				defer wg.Done()

				//Create a block of pixels for the thread to work on
				xStart := xBlock * workBlock
				yStart := yBlock * workBlock

				xEnd := xStart + workBlock
				yEnd := yStart + workBlock

				//Don't render off the screen
				if xStart < 0 {
					xStart = 0
				}
				if yStart < 0 {
					yStart = 0
				}
				if xEnd > imgWidth {
					xEnd = imgWidth
				}
				if yEnd > imgHeight {
					yEnd = imgHeight
				}

				//If the block is off the screen entirely, skip it
				if xStart > imgWidth {
					return
				}
				if yStart > imgHeight {
					return
				}

				//Render the block
				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						var pixel uint32 = 0
						var r, g, b uint32
						var sx, sy float64

						//Supersample
						for sx = 0; sx < superSample; sx++ {
							for sy = 0; sy < superSample; sy++ {
								//Get the sub-pixel position
								ssx := float64(sx) / float64(superSample)
								ssy := float64(sy) / float64(superSample)

								//Translate to position on the mandelbrot
								xx := ((((float64(x) + ssx) / imgWidth) - 0.5) / curZoom) - offX
								yy := ((((float64(y) + ssy) / imgWidth) - 0.3) / curZoom) - offY

								c := complex(xx, yy) //Rotate
								z := complex(0, 0)

								var it uint32 = 0
								skip := false

								//Pre-interate (no draw)
								//Speed + asthetic choice
								for i := 0; i < preIters; i++ {
									z = z*z + c
									if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
										skip = true
										break
									}
								}

								//Don't render at all if we escaped in the pre-iteration.
								if !skip {
									for it = 0; it < numIters; it++ {
										z = z*z + c
										if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
											break
										}
									}
								}

								//Don't render if we didn't escape
								//This allows background to be black
								if it > 0 {
									//Add the value ( gamma correct ) to the total
									//We later divide to get the average for super-sampling
									pixel += paletteL[it]

									tr, tg, tb := colorutil.HsvToRgb(float64((it*colorDegPerInter)%360), 1.0, 1.0)
									//We already gamma corrected, so use gamma 1.0 for chroma
									//But still convert from 8 bits to 16, to match the luma
									r += paletteC[tr]
									g += paletteC[tg]
									b += paletteC[tb]
								}
							}
						}

						//Add the pixel to the buffer, divide by number of samples for super-sampling
						offscreen.Set(int(x), int(y), color.RGBA64{
							uint16((r/numSamples)/2 + (pixel/numSamples)/2),
							uint16((g/numSamples)/2 + (pixel/numSamples)/2),
							uint16((b/numSamples)/2 + (pixel/numSamples)/2), 0xFFFF})
					}
				}
			}(xBlock, yBlock)
		}
	}
	wg.Wait()

	return true
}

func calcZoom() {
	zoomInt = zoomInt + zoomAdd
	sStep := zoomInt / zoomDiv
	curZoom = math.Pow(sStep, zoomPow)
}
