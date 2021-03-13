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
	screenWidth  = 1500
	screenHeight = 1500
	maxIt        = 10000
	gamma        = 0.3
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

	go func() {
		for {
			offscreen.ReplacePixels(offscreenPix)
			time.Sleep((1000 / fps) * time.Millisecond)
		}
	}()

	go func() {
		swg := sizedwaitgroup.New(numThreads)
		for j := 0; j < screenHeight; j++ {
			swg.Add()
			go func(j int) {
				defer swg.Done()
				for i := 0; i < screenWidth; i++ {
					x := float64(i)*size/screenHeight - size/2 + centerX
					y := (screenHeight-float64(j))*size/screenHeight - size/2 + centerY
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
	}()

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
	updateOffscreen(-0.75, 0.25, 2)
}
