package main

import (
	"encoding/binary"
	"fmt"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	spatial "git.sequentialread.com/forest/modular-spatial-index"
	leveldb "github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Query struct {
	X      int
	Y      int
	Width  int
	Height int
	Ranges []spatial.ByteRange
}

const numberOfKeys = float64(900000)
const numberOfQueries = 15000
const minQuerySizeInPixels = float64(0.1)
const maxQuerySizeInPixels = float64(1)
const tendencyToMakeSmallQueries = 1
const valueSizeBytes = 4096
const debugLog = false

func main() {
	// benchmarkHilbert(32, 0.1) // iopscostparam = 0.1, try to read fewer keys with more byte ranges
	// benchmarkHilbert(32, 1)
	benchmarkHilbert(64, 0.1) // iopscostparam = 0.1, try to read fewer keys with more byte ranges
	benchmarkHilbert(64, 1)
	benchmarkSliced(16)  // image is 512x512, so with 16 slices each slice is 32px tall.
	benchmarkSliced(32)  // with 32 slices each slice is 16px tall.
	benchmarkSliced(64)  // with 64 slices each slice is 8px tall.
	benchmarkSliced(128) // with 128 slices each slice is 4px tall.
}

func benchmarkHilbert(curveBits int, iopsCostParam float32) {
	benchmark(true, 0, curveBits, iopsCostParam)
}

func benchmarkSliced(sliceCount int) {
	benchmark(false, sliceCount, 64, 0)
}

func benchmark(hilbertMode bool, sliceCount int, curveBits int, iopsCostParam float32) {

	databaseFilename := fmt.Sprintf("db_hilbert-%t_curve-%d_%d-slices", hilbertMode, curveBits, sliceCount)

	db, err := leveldb.OpenFile(databaseFilename, &opt.Options{})
	defer (func() {
		err := db.Close()
		if err != nil {
			panic(err)
		}
	})()

	if err != nil {
		panic(err)
	}

	index, err := spatial.NewSpatialIndex2D(curveBits)
	if err != nil {
		panic(err)
	}
	indexMin, indexMax := index.GetValidInputRange()

	minKey := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	maxKey := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	sizes, err := db.SizeOf([]util.Range{{Start: minKey, Limit: maxKey}})
	if err != nil {
		panic(err)
	}

	file, err := os.OpenFile("densitymap.png", os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	image, err := png.Decode(file)
	if err != nil {
		panic(err)
	}
	totalBrightness := float64(0)
	imageBounds := image.Bounds()
	iterateImage := func(progress bool, forEachPixel func(int, int, float64) error) {
		lastPercent := 0
		for y := 0; y < imageBounds.Max.Y; y++ {
			for x := 0; x < imageBounds.Max.X; x++ {
				v, _, _, _ := image.At(x, y).RGBA()
				vf := float64(v) / float64(255)
				vf = vf * vf
				err := forEachPixel(x, y, vf)
				if err != nil {
					panic(err)
				}
			}
			percent := int((float64(y) / float64(imageBounds.Max.Y)) * 100)
			if progress && percent != lastPercent {
				log.Println(percent, "%")
				lastPercent = percent
			}
		}
	}
	pixelCoordsToIndexCoords := func(x, y int) (int, int) {
		xLerp := float64(x+1) / float64(imageBounds.Max.X+4)
		yLerp := float64(y+1) / float64(imageBounds.Max.Y+4)
		outX := int(lerp(float64(indexMin), float64(indexMax), xLerp))
		outY := int(lerp(float64(indexMin), float64(indexMax), yLerp))
		return outX, outY
	}
	pixelDimensionToIndexDimension := func(width, height float64) (int, int) {
		xLerp := width / float64(imageBounds.Max.X+4)
		yLerp := height / float64(imageBounds.Max.Y+4)
		outX := int(lerp(0, float64(indexMax)-float64(indexMin), xLerp))
		outY := int(lerp(0, float64(indexMax)-float64(indexMin), yLerp))
		return outX, outY
	}
	spatialKeyFromPoint := func(x, y int) ([]byte, error) {
		key, err := index.GetIndexedPoint(x, y)
		if err != nil {
			return nil, err
		}
		xBytes := make([]byte, 8)
		yBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(xBytes, uint64(x+indexMax))
		binary.BigEndian.PutUint64(yBytes, uint64(y+indexMax))
		return append(key, append(xBytes, yBytes...)...), nil
	}
	pointFromSpatialKey := func(key []byte) (int, int) {
		return int(binary.BigEndian.Uint64(key[8:16])) - indexMax, int(binary.BigEndian.Uint64(key[16:24])) - indexMax
	}
	naiveSpatialKeyFromPointWithSlice := func(slice, x, y int) []byte {
		sliceBytes := make([]byte, 2)
		xBytes := make([]byte, 8)
		yBytes := make([]byte, 8)
		binary.BigEndian.PutUint16(sliceBytes, uint16(int16(slice)))
		binary.BigEndian.PutUint64(xBytes, uint64(x+indexMax))
		binary.BigEndian.PutUint64(yBytes, uint64(y+indexMax))
		return append(sliceBytes, append(xBytes, yBytes...)...)
	}
	pointFromNaiveSpatialKey := func(key []byte) (x, y int) {
		return int(binary.BigEndian.Uint64(key[2:10])) - indexMax, int(binary.BigEndian.Uint64(key[10:18])) - indexMax
	}

	if sizes.Sum() == 0 {
		log.Printf("database %s appears to be empty, seeding it now...\n", databaseFilename)

		// count up the total brigtness in the image
		iterateImage(false, func(_ int, _ int, v float64) error {
			totalBrightness += v
			return nil
		})

		// insert [numberOfKeys total] keys into the db.
		// for each pixel in the image depending on the density at that pixel.
		realInserted := 0
		iterateImage(true, func(x int, y int, v float64) error {
			pixelX := x
			pixelY := y
			slice := int(math.Floor(lerp(float64(0), float64(sliceCount), float64(pixelY)/float64(imageBounds.Max.Y))))
			xMin, yMin := pixelCoordsToIndexCoords(x, y)
			xMax, yMax := pixelCoordsToIndexCoords(x+1, y+1)
			density := int(math.Round((v * numberOfKeys) / totalBrightness))
			for i := density; i > 0; i-- {
				x := int(lerp(float64(xMin), float64(xMax), rand.Float64()))
				y := int(lerp(float64(yMin), float64(yMax), rand.Float64()))
				var key []byte
				if hilbertMode {
					key, err = spatialKeyFromPoint(x, y)
					if err != nil {
						return err
					}
				} else {
					key = naiveSpatialKeyFromPointWithSlice(slice, x, y)
				}

				if debugLog {
					if (pixelY == 128 || pixelY == 256 || pixelY == 400) && (pixelX > 180 && pixelX < 236) || (pixelX > 333 && pixelX < 400) && i == 1 {
						fmt.Printf("%x   %d,%d   %d,%d  \n", key, x, y, pixelX, pixelY)
					}
				}

				value := make([]byte, valueSizeBytes)
				rand.Read(value)
				err = db.Put(key, value, &opt.WriteOptions{})
				realInserted++
				if err != nil {
					return err
				}
			}

			return nil
		})

		log.Printf("inserted %d keys\n", realInserted)

		// compact the entire DB.
		err = db.CompactRange(util.Range{})
		if err != nil {
			panic(err)
		}

		log.Println("CompactRange done!")

		err = db.Close()
		if err != nil {
			panic(err)
		}
		db, err = leveldb.OpenFile(databaseFilename, &opt.Options{})
		if err != nil {
			panic(err)
		}
	}

	sizes, err = db.SizeOf([]util.Range{{Start: minKey, Limit: maxKey}})
	if err != nil {
		panic(err)
	}
	log.Printf("database size: %d\n", sizes.Sum())

	queries := make([]Query, numberOfQueries)

	queryRand := rand.New(rand.NewSource(12903712398))
	for i := 0; i < numberOfQueries; i++ {
		widthPx := lerp(minQuerySizeInPixels, maxQuerySizeInPixels, math.Pow(queryRand.Float64(), tendencyToMakeSmallQueries))
		heightPx := lerp(minQuerySizeInPixels, maxQuerySizeInPixels, math.Pow(queryRand.Float64(), tendencyToMakeSmallQueries))
		if widthPx == 0 {
			widthPx = 1
		}
		if heightPx == 0 {
			heightPx = 1
		}
		yLerp := queryRand.Float64()
		xPx := int(lerp(float64(1), float64(imageBounds.Max.X)-widthPx, queryRand.Float64()))
		yPx := int(lerp(float64(1), float64(imageBounds.Max.Y)-heightPx, yLerp))
		x, y := pixelCoordsToIndexCoords(xPx, yPx)
		width, height := pixelDimensionToIndexDimension(widthPx, heightPx)
		var ranges []spatial.ByteRange
		if hilbertMode {
			ranges, err = index.RectangleToIndexedRanges(x, y, width, height, iopsCostParam)
			if err != nil {
				// xLerp := float64(widthPx) / float64(imageBounds.Max.X+4)
				// yLerp := float64(heightPx) / float64(imageBounds.Max.Y+4)
				// outX := int(lerp(0, float64(indexMax)-float64(indexMin), xLerp))
				// outY := int(lerp(0, float64(indexMax)-float64(indexMin), yLerp))
				// log.Printf("%d %d %.2f %.2f %d %d %d %d\n", widthPx, heightPx, xLerp, yLerp, width, height, outX, outY)
				// log.Printf("%d %d %d %d (%d..%d)\n", x, y, x+width, y+height, indexMin, indexMax)
				panic(fmt.Sprintf("%+v", err))
			}
		} else {

			heightInSlices := (float64(heightPx) / float64(imageBounds.Max.Y)) * float64(sliceCount)
			sliceFloat := lerp(0, float64(sliceCount)-heightInSlices, yLerp)
			minSlice := int(math.Floor(sliceFloat))
			maxSlice := int(math.Ceil(sliceFloat + heightInSlices))

			ranges = make([]spatial.ByteRange, maxSlice-minSlice)
			for j := 0; j < len(ranges); j++ {
				ranges[j] = spatial.ByteRange{
					Start: naiveSpatialKeyFromPointWithSlice(minSlice+j, x, y),
					End:   naiveSpatialKeyFromPointWithSlice(minSlice+j, x+width, y+height),
				}
			}

		}

		queries[i] = Query{
			X:      x,
			Y:      y,
			Width:  width,
			Height: height,
			Ranges: ranges,
		}

		if debugLog && i < 10 {
			fmt.Printf("%d\n%d\n%d\n%d\n------\n", xPx, yPx, widthPx, heightPx)
			fmt.Printf("%d\n%d\n%d\n%d\n", queries[i].X, queries[i].Y, queries[i].Width, queries[i].Height)
			for _, rng := range queries[i].Ranges {
				fmt.Printf("%x,\n%x\n\n", rng.Start, rng.End)
			}
			fmt.Println("---------\n")
		}
	}

	log.Printf("Generated %d queries\n", numberOfQueries)

	sumOfWastedKeysRatios := float64(0)
	sumOfRangeCounts := 0
	totalKeysFound := 0
	queryStartTime := time.Now()

	for i := 0; i < numberOfQueries; i++ {

		inRectangle := 0
		outsideOfRectangle := 0
		for _, rng := range queries[i].Ranges {
			iter := db.NewIterator(&util.Range{Start: rng.Start, Limit: rng.End}, nil)
			for iter.Next() {
				var foundX, foundY int
				if hilbertMode {
					foundX, foundY = pointFromSpatialKey(iter.Key())
				} else {
					foundX, foundY = pointFromNaiveSpatialKey(iter.Key())
				}

				if foundX > queries[i].X && foundY > queries[i].Y && foundX < queries[i].X+queries[i].Width && foundY < queries[i].Y+queries[i].Height {
					inRectangle++
					totalKeysFound += 1
				} else {
					outsideOfRectangle++
				}
			}
			iter.Release()
			err = iter.Error()
			if err != nil {
				panic(err)
			}
		}

		if inRectangle != 0 {
			sumOfWastedKeysRatios += float64(outsideOfRectangle) / float64(inRectangle)
		} else if outsideOfRectangle > 0 {
			sumOfWastedKeysRatios += 1
		}

		sumOfRangeCounts += len(queries[i].Ranges)
	}

	paramString := fmt.Sprintf("sliceCount: %d", sliceCount)
	if hilbertMode {
		paramString = fmt.Sprintf("iopsCostParam: %.2f", iopsCostParam)
	}

	log.Printf(
		"hilbertMode: %t, %s, took %s, average oversampling: %.2f, average range count: %.2f, totalKeysFound: %d\n",
		hilbertMode, paramString,
		time.Since(queryStartTime).String(),
		sumOfWastedKeysRatios/float64(numberOfQueries),
		float64(sumOfRangeCounts)/float64(numberOfQueries),
		totalKeysFound,
	)

}

func clamp01(x float64) float64 {
	if x > 1 {
		return float64(1)
	}
	if x < 0 {
		return float64(0)
	}
	return x
}

func lerp(a, b, lerp float64) float64 {
	lerp = clamp01(lerp)
	return a*(float64(1)-lerp) + b*lerp
}
