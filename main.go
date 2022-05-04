package main

import (
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
	liveUpdate  = false
	preIters    = 10
	maxIters    = 800
	chromaMode  = true
	autoZoom    = true
	startOffset = 9850

	winWidth  = 3840
	winHeight = 2160
	//This is the X/Y size, number of samples is superSample*superSample
	superSample = 8 //max 255
	endFrame    = 4500

	offX      = 0.747926709975882
	offY      = -0.10785035275635992
	zoomPow   = 100
	zoomDiv   = 10000.0
	escapeVal = 4.0
	zoomAdd   = 1

	gamma            = 0.8
	reportInterval   = 30 * time.Second
	workBlock        = 128
	colorDegPerInter = 4
)

var (
	palette      [((maxIters - preIters) + 256) + 1]uint32
	renderWidth  int = winWidth
	renderHeight int = winHeight

	offscreen *image.RGBA64

	numThreads = runtime.NumCPU()
	startTime  = time.Now()
	frameTime  = time.Now()

	curZoom         float64 = 1.0
	zoomInt         int     = startOffset
	lastReported    time.Time
	lastReportedVal float64
	frameCount      int
	pixelCount      uint64
	pixelCountTotal uint64
)

type Game struct {
}

func main() {
	lastReported = time.Now()
	startTime = time.Now()

	//Alloc images
	offscreen = image.NewRGBA64(image.Rect(0, 0, renderWidth, renderHeight))

	//Make gamma LUT
	for i := range palette {
		palette[i] = uint32(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xFFFF))
	}

	//Calculate zoom
	sStep := (float64(zoomInt) / zoomDiv)
	curZoom = (math.Pow(sStep, zoomPow))

	//Render loop
	for {
		rendered := updateOffscreen()
		if autoZoom {
			zoomInt = zoomInt + zoomAdd
			sStep := (float64(zoomInt) / zoomDiv)
			curZoom = (math.Pow(sStep, zoomPow))
		}

		//If we have a result (we can skip frames for resume)
		if rendered {
			if autoZoom {
				if chromaMode {

					fileName := fmt.Sprintf("out/color-%v.tif", frameCount)
					output, err := os.Create(fileName)
					opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
					if tiff.Encode(output, offscreen, opt) != nil {
						log.Println("ERROR: Failed to write image:", err)
						os.Exit(1)
					}
					output.Close()
				}

				if liveUpdate {
					/*Clear buffer after completed*/
					for x := 0; x < renderWidth; x++ {
						for y := 0; y < renderHeight; y++ {
							offscreen.Set(x, y, color.RGBA64{0, 0, 0, 0})
						}
					}
				}
			}
			fmt.Println("Completed frame:", frameCount)
		}
		if frameCount >= endFrame {
			os.Exit(0)
			return
		}
		frameCount += zoomAdd
	}
}

func updateOffscreen() bool {

	pixelCountTotal = 1
	pixelCount = 1
	frameTime = time.Now()
	time.Sleep(time.Millisecond)

	wg := sizedwaitgroup.New(numThreads)

	ss := uint32(superSample * superSample)
	numWorkBlocks := int(math.Ceil((float64(renderWidth) / float64(workBlock)) * (float64(renderHeight) / float64(workBlock))))
	blocksDone := 0
	lastReportedVal = 0

	if chromaMode {

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
	}

	for xBlock := 0; xBlock <= renderWidth/workBlock; xBlock++ {
		for yBlock := 0; yBlock <= renderHeight/workBlock; yBlock++ {
			blocksDone++
			percentDone := (float64(blocksDone) / float64(numWorkBlocks) * 100.0)

			if time.Since(lastReported) > reportInterval && lastReportedVal < percentDone {

				fmt.Printf("%v/%v: %0.2f%%, Work blocks(%d/%d) %vpps (%v)\n", time.Since(startTime).Round(time.Second).String(),
					time.Since(frameTime).Round(time.Second).String(), percentDone, blocksDone, numWorkBlocks,
					numToString(float64(pixelCount)/float64(time.Since(lastReported).Milliseconds()/1000.0)),
					numToString(float64(pixelCountTotal)/float64(time.Since(frameTime).Milliseconds()/1000.0)))

				lastReported = time.Now()
				lastReportedVal = percentDone
				pixelCount = 1
			}

			wg.Add()
			go func(xBlock, yBlock int) {
				defer wg.Done()

				xStart := int(math.Round(float64(xBlock * workBlock)))
				yStart := int(math.Round(float64(yBlock * workBlock)))

				xEnd := xStart + workBlock
				yEnd := yStart + workBlock

				if xStart < 0 {
					xStart = 0
				}
				if yStart < 0 {
					yStart = 0
				}
				if xEnd > renderWidth {
					xEnd = renderWidth
				}
				if yEnd > renderHeight {
					yEnd = renderHeight
				}
				if xStart > renderWidth {
					return
				}
				if yStart > renderHeight {
					return
				}

				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {

						var pixel uint32 = 0
						var r, g, b uint32

						for sx := 0; sx < superSample; sx++ {
							for sy := 0; sy < superSample; sy++ {
								ssx := float64(sx) / float64(superSample)
								ssx -= (superSample / 2.0)
								ssy := float64(sy) / float64(superSample)
								ssy -= (superSample / 2.0)

								xx := (((float64(x)+ssx)/float64(renderWidth) - 0.5) / (curZoom)) - offX
								yy := (((float64(y)+ssy)/float64(renderWidth) - 0.3) / (curZoom)) - offY

								t := iteratePoint(xx, yy)
								if t > 1 {
									if t < maxIters-preIters-1 {
										pixel += t

										tr, tg, tb := colorutil.HsvToRgb(math.Mod(float64(t)*colorDegPerInter, 360.0), 1.0, 1.0)
										r += uint32(tr)
										g += uint32(tg)
										b += uint32(tb)
									}
								}
							}
						}

						offscreen.Set(x, y, color.RGBA64{uint16(palette[(r/ss)+pixel/ss]), uint16(palette[(g/ss)+pixel/ss]), uint16(palette[(b/ss)+pixel/ss]), 0xFFFF})
					}
				}
				pps := (uint64(xEnd-xStart) * uint64(yEnd-yStart)) * (superSample * superSample)
				pixelCount += pps
				pixelCountTotal += pps
			}(xBlock, yBlock)
		}
		if liveUpdate {
			go func() {
				fileName := "out/live.tiff"
				output, _ := os.Create(fileName)
				opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
				tiff.Encode(output, offscreen, opt)
				output.Close()
			}()
		}

	}
	wg.Wait()

	return true
}

func iteratePoint(x, y float64) uint32 {

	c := complex(x, y) //Rotate
	z := complex(0, 0)

	var it uint32 = 0

	skip := false
	for i := 0; i < preIters; i++ {
		z = z*z + c
		if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
			skip = true
			break
		}
	}
	var iters uint32 = maxIters - preIters
	if !skip {
		for it = 0; it < iters; it++ {
			z = z*z + c
			if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
				break
			}
		}
	}
	return it

}

func numToString(num float64) string {
	if num > 1000 {
		return fmt.Sprintf("%0.2fk", float64(num)/1000.0)
	} else if num > 1000000 {
		return fmt.Sprintf("%0.2fM", float64(num)/1000000.0)
	} else if num > 1000000000 {
		return fmt.Sprintf("%0.2fG", float64(num)/1000000000.0)
	} else if num > 1000000000000 {
		return fmt.Sprintf("%0.2fT", float64(num)/1000000000000.0)
	}
	return fmt.Sprintf("%0.2f", float64(num))
}
