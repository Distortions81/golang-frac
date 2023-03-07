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
	iterCap  = 10000
	iterMin  = 100
	preIters = 10
)

var (
	disChroma        *bool
	disLuma          *bool
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
	numThreads       *int
	workBlock        *float64
	colorBrightness  *float64
	colorSaturation  *float64
	numInterations   *int
	doSleep          *bool
	sleepMicro       *int

	//Sleep this long before starting a new thread
	//Doesn't affect performance that much, but helps multitasking
	threadSleep time.Duration = time.Microsecond

	//Gamma LUT tables
	paletteL [iterCap]uint32
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

	DnumThreads := (runtime.NumCPU())

	disChroma = flag.Bool("disChroma", false, "Do not output chroma image")
	disLuma = flag.Bool("disLuma", false, "Do not output luma image")
	outDir = flag.String("outDir", "out", "output directory")
	imgWidth = flag.Float64("width", 3840, "Width of output image")
	imgHeight = flag.Float64("height", 2160, "Height of output image")
	superSample = flag.Float64("super", 8, "Super sampling x/y size")
	endFrame = flag.Float64("end", 3600, "Stop on this frame number")
	offX = flag.Float64("offx", 0, "X offset")
	offY = flag.Float64("offy", 0, "Y offset")
	zoomPow = flag.Float64("zoom", 100, "Zoom power")
	escapeVal = flag.Float64("escape", 4, "Escape value")
	gammaLuma = flag.Float64("gammaLuma", 1.0, "Luma gamma")
	gammaChroma = flag.Float64("gammaChroma", 1.0, "Chroma gamma")
	zoomAdd = flag.Float64("zoomAdd", 1, "Zoom step size")
	zSpeedDiv = flag.Float64("zSpeedDiv", 1.0, "Zoom speed divisor")
	colorDegPerInter = flag.Int("colorDegPerInter", 1, "Color degrees per iteration")
	numThreads = flag.Int("numThreads", DnumThreads, "Number of threads to use")
	workBlock = flag.Float64("workBlock", 64, "Work block size x/y size")
	colorBrightness = flag.Float64("colorBrightness", 0.5, "HSV brightness of the chroma")
	colorSaturation = flag.Float64("colorSaturation", 0.8, "HSV saturation of the chroma")
	numInterations = flag.Int("iters", 2500, "Max number of iterations")
	doSleep = flag.Bool("doSleep", false, "Sleep before work blocks")
	sleepMicro = flag.Int("sleepMicro", 100, "Microseconds of sleep before each workblock")
	flag.Parse()

	threadSleep = time.Duration(*sleepMicro)

	fmt.Printf("%v,%v\n", *offX, *offY)

	/* Statically allocated */
	if *numInterations > iterCap {
		a := iterCap
		numInterations = &a
	} else if *numInterations < iterMin {
		a := iterMin
		numInterations = &a
	}
	if *superSample < 1 {
		a := 1.0
		superSample = &a
	} else if *superSample > 255 {
		a := 255.0
		superSample = &a
	}

	//zoom step size
	zoomDiv = 10000.0 / *zSpeedDiv
	//Integer zoom is based on
	zoomInt = 9800.0 / *zSpeedDiv

	//Alloc images
	offscreen = image.NewGray16(image.Rect(0, 0, int(*imgWidth), int(*imgHeight)))
	offscreenC = image.NewRGBA(image.Rect(0, 0, int(*imgWidth), int(*imgHeight)))

	//Setup
	wg = sizedwaitgroup.New(*numThreads)
	numSamples = uint32(int(*superSample) * int(*superSample))
	numIters = uint32(*numInterations) - preIters

	//Make gamma LUTs
	var i uint32
	for i = 0; i < numIters; i++ {
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
		renderStart := time.Now()
		rendered := updateOffscreen()
		took := time.Since(renderStart).Seconds()

		//Update zoom for next frame
		calcZoom()

		//If we have a result, write it
		//(we can skip frames for resume and multi-machine rendering)
		if rendered {

			wg.Add()
			go func() {

				if !*disChroma {
					fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "chroma", frameCount)
					output, err := os.Create(fileName)
					opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
					if tiff.Encode(output, offscreenC, opt) != nil {
						log.Println("ERROR: Failed to write image:", err)
						os.Exit(1)
					}
					output.Close()
				}
				wg.Done()
			}()

			wg.Add()
			go func() {
				if !*disLuma {
					fileName := fmt.Sprintf("%v/%v-%v.tif", *outDir, "luma", frameCount)
					output, err := os.Create(fileName)
					opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
					if tiff.Encode(output, offscreen, opt) != nil {
						log.Println("ERROR: Failed to write image:", err)
						os.Exit(1)
					}
					output.Close()
				}
				wg.Done()
			}()

			wg.Wait()

			remain := *endFrame - frameCount
			eta := time.Duration(took*remain) * time.Second
			fmt.Printf("Completed frame: %v / %v (%v remaining) ETA: %v\n", frameCount, *endFrame, remain, eta.String())
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
	if !*disChroma {
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
	if !*disLuma {
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
			if *doSleep {
				//Give process manager a moment
				time.Sleep(threadSleep * time.Microsecond)
			}
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
								xx := ((((float64(x) + ssx) / *imgWidth) - 0.5) / curZoom) - (*offX)
								yy := ((((float64(y) + ssy) / *imgWidth) - 0.3) / curZoom) - (*offY)

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

									tr, tg, tb := colorutil.HsvToRgb(float64((it)*uint32(*colorDegPerInter)%360), *colorSaturation, *colorBrightness)
									//We already gamma corrected, so use gamma 1.0 for chroma
									//But still convert from 8 bits to 16, to match the luma
									r += paletteC[tr]
									g += paletteC[tg]
									b += paletteC[tb]
								}
							}
						}

						if !*disLuma {
							offscreen.Set(int(x), int(y), color.Gray16{uint16(pixel / numSamples)})
						}
						if !*disChroma {
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
