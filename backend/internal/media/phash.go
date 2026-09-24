package media

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/bits"
	"os"
	"strconv"
)

// ComputePHash computes a 64-bit DCT perceptual hash from an image file path.
// Returns a 16-character hexadecimal string representing the 64-bit hash.
func ComputePHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open image for phash: %w", err)
	}
	defer f.Close()

	return ComputePHashFromReader(f)
}

// ComputePHashFromReader decodes an image stream and computes its DCT perceptual hash.
func ComputePHashFromReader(r io.Reader) (string, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	const (
		sampleSize = 32
		dctSize    = 8
	)

	// 1. Resize to 32x32 grayscale
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return "", fmt.Errorf("invalid image dimensions: %dx%d", srcW, srcH)
	}

	gray := make([][]float64, sampleSize)
	for y := 0; y < sampleSize; y++ {
		gray[y] = make([]float64, sampleSize)
		for x := 0; x < sampleSize; x++ {
			srcX := bounds.Min.X + (x*srcW)/sampleSize
			srcY := bounds.Min.Y + (y*srcH)/sampleSize
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			// Standard ITU-R BT.601 luma formula (values scaled 0..255)
			luma := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			gray[y][x] = luma
		}
	}

	// 2. Compute 2D Discrete Cosine Transform (DCT) on 32x32
	dct := make([][]float64, sampleSize)
	for u := 0; u < sampleSize; u++ {
		dct[u] = make([]float64, sampleSize)
		for v := 0; v < sampleSize; v++ {
			var sum float64
			for i := 0; i < sampleSize; i++ {
				for j := 0; j < sampleSize; j++ {
					sum += gray[i][j] *
						math.Cos(((2*float64(i)+1)*float64(u)*math.Pi)/(2*float64(sampleSize))) *
						math.Cos(((2*float64(j)+1)*float64(v)*math.Pi)/(2*float64(sampleSize)))
				}
			}
			alphaU := 1.0
			if u == 0 {
				alphaU = 1.0 / math.Sqrt(2)
			}
			alphaV := 1.0
			if v == 0 {
				alphaV = 1.0 / math.Sqrt(2)
			}
			dct[u][v] = 0.25 * (2.0 / float64(sampleSize)) * alphaU * alphaV * sum
		}
	}

	// 3. Extract top-left 8x8 DCT (excluding DC term [0][0])
	var vals []float64
	for u := 0; u < dctSize; u++ {
		for v := 0; v < dctSize; v++ {
			if u == 0 && v == 0 {
				continue
			}
			vals = append(vals, dct[u][v])
		}
	}

	// 4. Calculate median
	var sum float64
	for _, v := range vals {
		sum += v
	}
	avg := sum / float64(len(vals))

	// 5. Construct 64-bit hash
	var hash uint64
	bitIdx := 0
	for u := 0; u < dctSize; u++ {
		for v := 0; v < dctSize; v++ {
			if u == 0 && v == 0 {
				continue
			}
			if dct[u][v] > avg {
				hash |= (1 << bitIdx)
			}
			bitIdx++
		}
	}

	return fmt.Sprintf("%016x", hash), nil
}

// HammingDistance calculates the bitwise Hamming distance between two 16-hex perceptual hashes.
// Returns -1 if either hash is malformed. A distance <= 10 typically indicates high visual similarity.
func HammingDistance(hash1, hash2 string) int {
	if len(hash1) != 16 || len(hash2) != 16 {
		return -1
	}
	u1, err1 := strconv.ParseUint(hash1, 16, 64)
	u2, err2 := strconv.ParseUint(hash2, 16, 64)
	if err1 != nil || err2 != nil {
		return -1
	}
	return bits.OnesCount64(u1 ^ u2)
}
