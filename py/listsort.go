// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// A port of CPython 3.14's listsort (Objects/listobject.c): timsort with the
// powersort merge policy. With an inconsistent "<" (NaN, random comparators)
// the result depends on the exact sequence of comparisons, so the algorithm is
// followed step for step; every function mirrors the C function of that name.

const (
	minGallop = 7
	maxMinrun = 64
)

type sortItem struct{ k, v Object }

type sortRun struct{ base, len, power int }

type sorter struct {
	ps        []sortItem
	tmp       []sortItem
	lt        func(a, b Object) (bool, error)
	err       error
	minGallop int
	pending   []sortRun
}

// islt is ISLT: 1 if a < b, 0 if not, -1 on error (kept in m.err).
func (m *sorter) islt(a, b Object) int {
	r, err := m.lt(a, b)
	if err != nil {
		m.err = err
		return -1
	}
	if r {
		return 1
	}
	return 0
}

func reverseItems(a []sortItem) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func (m *sorter) binarysort(a []sortItem, n, ok int) int {
	if ok == 0 {
		ok++
	}
	for ; ok < n; ok++ {
		l, r := 0, ok
		pivot := a[ok]
		for {
			p := (l + r) >> 1
			k := m.islt(pivot.k, a[p].k)
			if k < 0 {
				return -1
			}
			if k > 0 {
				r = p
			} else {
				l = p + 1
			}
			if l >= r {
				break
			}
		}
		copy(a[l+1:ok+1], a[l:ok])
		a[l] = pivot
	}
	return 0
}

func (m *sorter) countRun(a []sortItem, nr int) int {
	n := 1
	for ; n < nr; n++ {
		k := m.islt(a[n].k, a[n-1].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			break
		}
	}
	if n == nr {
		return n
	}
	if n > 1 {
		k := m.islt(a[0].k, a[n-1].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			return n
		}
		reverseItems(a[:n])
	}
	n++
	neq := 0
	revNeq := func() {
		if neq > 0 {
			neq++
			reverseItems(a[n-neq : n])
			neq = 0
		}
	}
	for ; n < nr; n++ {
		k := m.islt(a[n].k, a[n-1].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			revNeq()
			continue
		}
		k = m.islt(a[n-1].k, a[n].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			break
		}
		neq++
	}
	revNeq()
	reverseItems(a[:n])
	for ; n < nr; n++ {
		k := m.islt(a[n].k, a[n-1].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			break
		}
	}
	return n
}

func (m *sorter) gallopLeft(key Object, a []sortItem, n, hint int) int {
	lastofs, ofs := 0, 1
	k := m.islt(a[hint].k, key)
	if k < 0 {
		return -1
	}
	if k > 0 {
		maxofs := n - hint
		for ofs < maxofs {
			k = m.islt(a[hint+ofs].k, key)
			if k < 0 {
				return -1
			}
			if k == 0 {
				break
			}
			lastofs = ofs
			ofs = ofs<<1 + 1
		}
		if ofs > maxofs {
			ofs = maxofs
		}
		lastofs += hint
		ofs += hint
	} else {
		maxofs := hint + 1
		for ofs < maxofs {
			k = m.islt(a[hint-ofs].k, key)
			if k < 0 {
				return -1
			}
			if k > 0 {
				break
			}
			lastofs = ofs
			ofs = ofs<<1 + 1
		}
		if ofs > maxofs {
			ofs = maxofs
		}
		lastofs, ofs = hint-ofs, hint-lastofs
	}
	lastofs++
	for lastofs < ofs {
		mid := lastofs + (ofs-lastofs)>>1
		k = m.islt(a[mid].k, key)
		if k < 0 {
			return -1
		}
		if k > 0 {
			lastofs = mid + 1
		} else {
			ofs = mid
		}
	}
	return ofs
}

func (m *sorter) gallopRight(key Object, a []sortItem, n, hint int) int {
	lastofs, ofs := 0, 1
	k := m.islt(key, a[hint].k)
	if k < 0 {
		return -1
	}
	if k > 0 {
		maxofs := hint + 1
		for ofs < maxofs {
			k = m.islt(key, a[hint-ofs].k)
			if k < 0 {
				return -1
			}
			if k == 0 {
				break
			}
			lastofs = ofs
			ofs = ofs<<1 + 1
		}
		if ofs > maxofs {
			ofs = maxofs
		}
		lastofs, ofs = hint-ofs, hint-lastofs
	} else {
		maxofs := n - hint
		for ofs < maxofs {
			k = m.islt(key, a[hint+ofs].k)
			if k < 0 {
				return -1
			}
			if k > 0 {
				break
			}
			lastofs = ofs
			ofs = ofs<<1 + 1
		}
		if ofs > maxofs {
			ofs = maxofs
		}
		lastofs += hint
		ofs += hint
	}
	lastofs++
	for lastofs < ofs {
		mid := lastofs + (ofs-lastofs)>>1
		k = m.islt(key, a[mid].k)
		if k < 0 {
			return -1
		}
		if k > 0 {
			ofs = mid
		} else {
			lastofs = mid + 1
		}
	}
	return ofs
}

const (
	mergeSucceed = iota
	mergeFail
	mergeCopy
)

// mergeLo merges the runs ps[ia:ia+na] and ps[ia+na:ia+na+nb], na <= nb.
func (m *sorter) mergeLo(ia, na, nb int) int {
	ps := m.ps
	tmp := append(m.tmp[:0], ps[ia:ia+na]...)
	m.tmp = tmp
	dest, a, b := ia, 0, ia+na
	ps[dest] = ps[b]
	dest++
	b++
	nb--
	code := func() int {
		if nb == 0 {
			return mergeSucceed
		}
		if na == 1 {
			return mergeCopy
		}
		mg := m.minGallop
		for {
			acount, bcount := 0, 0
			for {
				k := m.islt(ps[b].k, tmp[a].k)
				if k < 0 {
					return mergeFail
				}
				if k > 0 {
					ps[dest] = ps[b]
					dest++
					b++
					bcount++
					acount = 0
					nb--
					if nb == 0 {
						return mergeSucceed
					}
					if bcount >= mg {
						break
					}
				} else {
					ps[dest] = tmp[a]
					dest++
					a++
					acount++
					bcount = 0
					na--
					if na == 1 {
						return mergeCopy
					}
					if acount >= mg {
						break
					}
				}
			}
			mg++
			for {
				if mg > 1 {
					mg--
				}
				m.minGallop = mg
				k := m.gallopRight(ps[b].k, tmp[a:], na, 0)
				acount = k
				if k != 0 {
					if k < 0 {
						return mergeFail
					}
					copy(ps[dest:dest+k], tmp[a:a+k])
					dest += k
					a += k
					na -= k
					if na == 1 {
						return mergeCopy
					}
					if na == 0 {
						return mergeSucceed
					}
				}
				ps[dest] = ps[b]
				dest++
				b++
				nb--
				if nb == 0 {
					return mergeSucceed
				}
				k = m.gallopLeft(tmp[a].k, ps[b:], nb, 0)
				bcount = k
				if k != 0 {
					if k < 0 {
						return mergeFail
					}
					copy(ps[dest:dest+k], ps[b:b+k])
					dest += k
					b += k
					nb -= k
					if nb == 0 {
						return mergeSucceed
					}
				}
				ps[dest] = tmp[a]
				dest++
				a++
				na--
				if na == 1 {
					return mergeCopy
				}
				if acount < minGallop && bcount < minGallop {
					break
				}
			}
			mg++
			m.minGallop = mg
		}
	}()
	if code == mergeCopy {
		copy(ps[dest:dest+nb], ps[b:b+nb])
		ps[dest+nb] = tmp[a]
		return 0
	}
	if na > 0 {
		copy(ps[dest:dest+na], tmp[a:a+na])
	}
	if code == mergeFail {
		return -1
	}
	return 0
}

// mergeHi is mergeLo for na >= nb, filling from the right.
func (m *sorter) mergeHi(ia, na, nb int) int {
	ps := m.ps
	ib := ia + na
	tmp := append(m.tmp[:0], ps[ib:ib+nb]...)
	m.tmp = tmp
	dest, a, b := ib+nb-1, ia+na-1, nb-1
	ps[dest] = ps[a]
	dest--
	a--
	na--
	code := func() int {
		if na == 0 {
			return mergeSucceed
		}
		if nb == 1 {
			return mergeCopy
		}
		mg := m.minGallop
		for {
			acount, bcount := 0, 0
			for {
				k := m.islt(tmp[b].k, ps[a].k)
				if k < 0 {
					return mergeFail
				}
				if k > 0 {
					ps[dest] = ps[a]
					dest--
					a--
					acount++
					bcount = 0
					na--
					if na == 0 {
						return mergeSucceed
					}
					if acount >= mg {
						break
					}
				} else {
					ps[dest] = tmp[b]
					dest--
					b--
					bcount++
					acount = 0
					nb--
					if nb == 1 {
						return mergeCopy
					}
					if bcount >= mg {
						break
					}
				}
			}
			mg++
			for {
				if mg > 1 {
					mg--
				}
				m.minGallop = mg
				k := m.gallopRight(tmp[b].k, ps[ia:], na, na-1)
				if k < 0 {
					return mergeFail
				}
				k = na - k
				acount = k
				if k != 0 {
					dest -= k
					a -= k
					copy(ps[dest+1:dest+1+k], ps[a+1:a+1+k])
					na -= k
					if na == 0 {
						return mergeSucceed
					}
				}
				ps[dest] = tmp[b]
				dest--
				b--
				nb--
				if nb == 1 {
					return mergeCopy
				}
				k = m.gallopLeft(ps[a].k, tmp, nb, nb-1)
				if k < 0 {
					return mergeFail
				}
				k = nb - k
				bcount = k
				if k != 0 {
					dest -= k
					b -= k
					copy(ps[dest+1:dest+1+k], tmp[b+1:b+1+k])
					nb -= k
					if nb == 1 {
						return mergeCopy
					}
					if nb == 0 {
						return mergeSucceed
					}
				}
				ps[dest] = ps[a]
				dest--
				a--
				na--
				if na == 0 {
					return mergeSucceed
				}
				if acount < minGallop && bcount < minGallop {
					break
				}
			}
			mg++
			m.minGallop = mg
		}
	}()
	if code == mergeCopy {
		copy(ps[dest+1-na:dest+1], ps[a+1-na:a+1])
		dest -= na
		ps[dest] = tmp[b]
		return 0
	}
	if nb > 0 {
		copy(ps[dest-(nb-1):dest+1], tmp[:nb])
	}
	if code == mergeFail {
		return -1
	}
	return 0
}

func (m *sorter) mergeAt(i int) int {
	p := m.pending
	ia, na := p[i].base, p[i].len
	ib, nb := p[i+1].base, p[i+1].len
	p[i].len = na + nb
	if i == len(p)-3 {
		p[i+1] = p[i+2]
	}
	m.pending = p[:len(p)-1]
	k := m.gallopRight(m.ps[ib].k, m.ps[ia:], na, 0)
	if k < 0 {
		return -1
	}
	ia += k
	na -= k
	if na == 0 {
		return 0
	}
	nb = m.gallopLeft(m.ps[ia+na-1].k, m.ps[ib:], nb, nb-1)
	if nb <= 0 {
		return nb
	}
	if na <= nb {
		return m.mergeLo(ia, na, nb)
	}
	return m.mergeHi(ia, na, nb)
}

func powerloop(s1, n1, n2, n int) int {
	result := 0
	a := 2*s1 + n1
	b := a + n1 + n2
	for {
		result++
		if a >= n {
			a -= n
			b -= n
		} else if b >= n {
			break
		}
		a <<= 1
		b <<= 1
	}
	return result
}

func (m *sorter) foundNewRun(n2 int) int {
	if len(m.pending) > 0 {
		p := m.pending
		top := &p[len(p)-1]
		power := powerloop(top.base, top.len, n2, len(m.ps))
		for len(m.pending) > 1 && m.pending[len(m.pending)-2].power > power {
			if m.mergeAt(len(m.pending)-2) < 0 {
				return -1
			}
		}
		m.pending[len(m.pending)-1].power = power
	}
	return 0
}

func (m *sorter) mergeForceCollapse() int {
	for len(m.pending) > 1 {
		n := len(m.pending) - 2
		if n > 0 && m.pending[n-1].len < m.pending[n+1].len {
			n--
		}
		if m.mergeAt(n) < 0 {
			return -1
		}
	}
	return 0
}

func mergeComputeMinrun(n int) int {
	r := 0
	for n >= maxMinrun {
		r |= n & 1
		n >>= 1
	}
	return n + r
}

// timsort sorts ps in place by key, stably, with CPython's exact comparison
// sequence. On error ps is still a permutation of its input.
func timsort(ps []sortItem, lt func(a, b Object) (bool, error)) error {
	m := &sorter{ps: ps, lt: lt, minGallop: minGallop}
	nr := len(ps)
	if nr < 2 {
		return nil
	}
	minrun := mergeComputeMinrun(nr)
	lo := 0
	for nr > 0 {
		n := m.countRun(ps[lo:], nr)
		if n < 0 {
			return m.err
		}
		if n < minrun {
			force := min(nr, minrun)
			if m.binarysort(ps[lo:], force, n) < 0 {
				return m.err
			}
			n = force
		}
		if m.foundNewRun(n) < 0 {
			return m.err
		}
		m.pending = append(m.pending, sortRun{base: lo, len: n})
		lo += n
		nr -= n
	}
	if m.mergeForceCollapse() < 0 {
		return m.err
	}
	return nil
}
