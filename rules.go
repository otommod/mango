package main

type AndRule []Rule

func (r AndRule) Block(m Metadata) bool {
	for _, x := range r {
		if x.Block(m) {
			return true
		}
	}
	return false
}

type LastChapterRule empty

func (LastChapterRule) Block(m Metadata) bool {
	return m["chapterIndex"].(int) < m["chapters"].(int)
}
