package models

import (
	"math"
	"time"
)

// represents the statistical confidence
//var StatisticalConfidence = 1.0 => ~69%, 1.96 => ~95% (default)
var StatisticalConfidence = 1.94

// represents how fast elapsed hours affect the order of an item
var HNGravity = 1.2

// wilson score interval sort
// http://www.evanmiller.org/how-not-to-sort-by-average-rating.html
func Wilson(ups, downs int64) float64 {
	n := ups + downs
	if n == 0 {
		return 0
	}

	n1 := float64(n)
	z := StatisticalConfidence
	p := float64(ups / n)
	zzfn := z * z / (4 * n1)
	w := (p + 2.0*zzfn - z*math.Sqrt((zzfn/n1+p*(1.0-p))/n1)) / (1 + 4*zzfn)

	return w
}

// hackernews' hot sort
// http://amix.dk/blog/post/19574
func Hacker(votes int64, date time.Duration) float64 {
	hoursAge := date.Hours()
	return float64(votes-1) / math.Pow(hoursAge+2, HNGravity)
}

// reddit's hot sort
// http://amix.dk/blog/post/19588
func Reddit(ups, downs int64, date time.Duration) float64 {
	decay := 45000.0
	s := float64(ups - downs)
	order := math.Log(math.Max(math.Abs(s), 1)) / math.Ln10
	return order - date.Seconds()/float64(decay)
}
