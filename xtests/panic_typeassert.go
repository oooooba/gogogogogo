package main

func main() {
	var i interface{} = 5
	_ = i.(string)
}
