package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/remeh/sizedwaitgroup"
)

const (
	screenWidth  = 1024
	screenHeight = 1024
	maxIters     = 1000
	offX         = -0.34831493420245574
	offY         = 0.606486596104741
	zoomSpeed    = 0.1
	gamma        = 0.6
)

var curZoom float64 = 1
var zoomInt int = 2

type Game struct{}

func (g *Game) Update() error {
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.DrawImage(offscreen, nil)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

var (
	offscreen    *ebiten.Image
	offscreenPix []byte
	palette      [maxIters]byte
	numThreads   = runtime.NumCPU()
)

func color(it int) (r, g, b byte) {
	if it == maxIters {
		return 0xff, 0xff, 0xff
	}
	c := palette[it]
	return c, c, c
}

func updateOffscreen(centerX, centerY, size float64) {

	swg := sizedwaitgroup.New(numThreads)
	for j := 0; j < screenHeight; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < screenWidth; i++ {
				fi := float64(i)
				fj := float64(j)
				x := (fj/screenHeight-0.5)/size*3.0 + centerX
				y := (fi/screenWidth-0.5)/size*3.0 + centerY
				c := complex(x, y) //Rotate
				z := complex(0, 0)
				it := 0
				for ; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > 4 {
						break
					}
				}
				r, g, b := color(it)
				p := 4 * (i + j*screenWidth)
				offscreenPix[p] = r
				offscreenPix[p+1] = g
				offscreenPix[p+2] = b
				offscreenPix[p+3] = 0xff
			}

		}(j)
	}
	swg.Wait()
	offscreen.ReplacePixels(offscreenPix)
	time.Sleep(time.Millisecond)

}

func init() {
	fmt.Printf("Allocating image...")
	offscreen = ebiten.NewImage(screenWidth, screenHeight)
	offscreenPix = make([]byte, screenWidth*screenHeight*4)
	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New((numThreads))
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()
			palette[i] = byte((math.Pow(float64(i)/float64(maxIters+1), gamma) * 0xff))
		}(i)
	}

	swg.Wait()
	fmt.Printf("complete!\n")
	buf := fmt.Sprintf("Threads found: %d", numThreads)
	fmt.Println(buf)

	go func() {
		for {
			updateOffscreen(offX, offY, curZoom)
			zoomInt = zoomInt + 1
			sStep := float64(zoomInt) / 1000.0
			curZoom = curZoom + (sStep * sStep * sStep * float64(zoomSpeed))

			//buf := fmt.Sprintf("%f", curZoom)
			//fmt.Println(buf)
		}
	}()
}
