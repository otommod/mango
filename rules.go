package main

type AndRule []Rule

func (r AndRule) Block(resrc Resource) bool {
	for _, x := range r {
		if x.Block(resrc) {
			return true
		}
	}
	return false
}

type LastChapterRule empty

func (LastChapterRule) Block(r Resource) bool {
	return r.info["chapterIndex"].(int) < r.info["chapters"].(int)
}

type funcRule func(Resource) bool

func (f funcRule) Block(r Resource) bool {
	return f(r)
}
