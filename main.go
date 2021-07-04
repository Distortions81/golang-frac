package main

import (
	"fmt"
	"log"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten"
	"github.com/remeh/sizedwaitgroup"
)

var zoom float64 = 0
var zoomCount int = 0

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

const (
	screenWidth  = 1024
	screenHeight = 1024
	maxIt        = 2000
	gamma        = 0.33333333
	fps          = 4
)

var (
	offscreen    *ebiten.Image
	offscreenPix []byte
	palette      [maxIt]byte
	numThreads   = runtime.NumCPU()
)

func color(it int) (r, g, b byte) {
	if it == maxIt {
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
				x := (fi/screenHeight-0.5)/size*3.0 - 0.7
				y := (fj/screenWidth - 0.5) / size * 3.0
				c := complex(x, y)
				z := complex(0, 0)
				it := 0
				for ; it < maxIt; it++ {
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

}

func init() {
	fmt.Printf("Allocating image...")
	offscreen = ebiten.NewImage(screenWidth, screenHeight)
	offscreenPix = make([]byte, screenWidth*screenHeight*4)
	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New(numThreads)
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()
			palette[i] = byte((math.Pow(float64(i)/float64(maxIt+1), gamma) * 0xff))
		}(i)
	}

	swg.Wait()
	fmt.Printf("complete!\n")

	go func() {
		for {
			updateOffscreen(0.0, 0.0, zoom)
			zoomCount = zoomCount + 1

			zoom = zoom + (0.08 / math.Sqrt(float64(zoomCount)))
			fmt.Println(zoom)
		}
	}()
}
