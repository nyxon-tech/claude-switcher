package i18n

import "time"

// breaks are the Jalali years where the 33-year leap cycle shifts (Borkowski), as in jalaali-js.
var breaks = [...]int{-61, 9, 38, 199, 426, 686, 756, 818, 1111, 1181, 1210, 1635, 2060, 2097,
	2192, 2262, 2324, 2394, 2456, 3178}

// jalali converts a Gregorian date to the Jalali (Solar Hijri) calendar. It is exact for
// Jalali years -61 to 3177; ok is false outside them.
func jalali(gy int, gm time.Month, gd int) (jy, jm, jd int, ok bool) {
	day := time.Date(gy, gm, gd, 0, 0, 0, 0, time.UTC)
	jy = gy - 621
	start, ok := nowruz(jy)
	if ok && day.Before(start) {
		jy--
		start, ok = nowruz(jy)
	}
	if !ok {
		return 0, 0, 0, false
	}
	k := int(day.Sub(start) / (24 * time.Hour))
	if k < 186 { // six months of 31 days
		return jy, 1 + k/31, 1 + k%31, true
	}
	k -= 186 // then 30-day months; Esfand has 29 or 30
	return jy, 7 + k/30, 1 + k%30, true
}

// nowruz is 1 Farvardin of Jalali year jy, a day in March of Gregorian year jy+621.
func nowruz(jy int) (time.Time, bool) {
	if jy < breaks[0] || jy >= breaks[len(breaks)-1] {
		return time.Time{}, false
	}
	// Count Jalali leap years since AD 621 up to jy.
	leapJ, jp, jump := -14, breaks[0], 0
	for _, jm := range breaks[1:] {
		jump = jm - jp
		if jy < jm {
			break
		}
		leapJ += jump/33*8 + jump%33/4
		jp = jm
	}
	n := jy - jp
	leapJ += n/33*8 + (n%33+3)/4
	if jump%33 == 4 && jump-n == 4 {
		leapJ++
	}
	// And Gregorian leap years up to the same year.
	gy := jy + 621
	leapG := gy/4 - (gy/100+1)*3/4 - 150
	return time.Date(gy, time.March, 20+leapJ-leapG, 0, 0, 0, 0, time.UTC), true
}
