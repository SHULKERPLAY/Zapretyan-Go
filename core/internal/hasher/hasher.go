package hasher // Helper for sream computing of SHA-256
import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
)

// CompareFilesHash calculate SHA-256 hashes and compare them.
// Return true, if files identical.
func CompareFilesHash(path1, path2 string) (bool, error) {
	defer slog.Debug("CompareFilesHash() ended")

	// If size of files different then they different. Skip hash check
	info1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	info2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}

	if info1.Size() != info2.Size() {
		slog.Debug("Files of different sizes. No need to compute hashes", "size1", info1.Size(), "size2", info2.Size())
		return false, nil
	}

	// Calculating hash of first file
	hash1, err := СalculateSHA256(path1)
	if err != nil {
		return false, err
	}

	// Calculating hash of second file
	hash2, err := СalculateSHA256(path2)
	if err != nil {
		return false, err
	}

	// Comparing hashes string
	slog.Debug("Hashes compared", "file1_sha", hash1, "file2_sha", hash2)
	return hash1 == hash2, nil
}

// Calculate SHA-256 of file
func СalculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()

	// io.Copy reads file by chunks with 32kb buffer
	// and send them to hasher reducing RAM consumption
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	// Get hash bytes and convert it to Hex string
	return hex.EncodeToString(hasher.Sum(nil)), nil
}