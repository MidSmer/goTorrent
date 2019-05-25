package torrent

import (
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/sirupsen/logrus"
	"io"
	"os"
)

func secondsToMinutes(inSeconds int64) string {
	minutes := inSeconds / 60
	seconds := inSeconds % 60
	minutesString := fmt.Sprintf("%d", minutes)
	secondsString := fmt.Sprintf("%d", seconds)
	str := minutesString + " Min/ " + secondsString + " Sec"
	return str
}

//VerifyData just verifies the data of a torrent by hash
func VerifyData(singleTorrent *torrent.Torrent) {
	singleTorrent.VerifyData()
}

//MakeRange creates a range of pieces to set their priority based on a file
func MakeRange(min, max int) []int {
	a := make([]int, max-min+1)
	for i := range a {
		a[i] = min + i
	}
	return a
}

//HumanizeBytes returns a nice humanized version of bytes in either GB or MB
func HumanizeBytes(bytes float32) string {
	if bytes < 1000000 { //if we have less than 1MB in bytes convert to KB
		pBytes := fmt.Sprintf("%.2f", bytes/1024)
		pBytes = pBytes + " KB"
		return pBytes
	}
	bytes = bytes / 1024 / 1024 //Converting bytes to a useful measure
	if bytes > 1024 {
		pBytes := fmt.Sprintf("%.2f", bytes/1024)
		pBytes = pBytes + " GB"
		return pBytes
	}
	pBytes := fmt.Sprintf("%.2f", bytes) //If not too big or too small leave it as MB
	pBytes = pBytes + " MB"
	return pBytes
}

//CopyFile takes a source file string and a destination file string and copies the file
func CopyFile(srcFile string, destFile string) { //TODO move this to our imported copy repo
	fileContents, err := os.Open(srcFile)
	defer fileContents.Close()
	if err != nil {
		Logger.WithFields(logrus.Fields{"File": srcFile, "Error": err}).Error("Cannot open source file")
	}
	outfileContents, err := os.Create(destFile)
	defer outfileContents.Close()
	if err != nil {
		Logger.WithFields(logrus.Fields{"File": destFile, "Error": err}).Error("Cannot open destination file")
	}
	_, err = io.Copy(outfileContents, fileContents)
	if err != nil {
		Logger.WithFields(logrus.Fields{"Source File": srcFile, "Destination File": destFile, "Error": err}).Error("Cannot write contents to destination file")
	}

}
