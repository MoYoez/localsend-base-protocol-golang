package tool

import (
	"fmt"
	"math/rand"
)

// I assume that it"s not useful, :Wink /

var adjectives = []string{
	"Adorable",
	"Beautiful",
	"Big",
	"Bright",
	"Clean",
	"Clever",
	"Cool",
	"Cute",
	"Cunning",
	"Determined",
	"Energetic",
	"Efficient",
	"Fantastic",
	"Fast",
	"Fine",
	"Fresh",
	"Good",
	"Gorgeous",
	"Great",
	"Handsome",
	"Hot",
	"Kind",
	"Lovely",
	"Mystic",
	"Neat",
	"Nice",
	"Patient",
	"Pretty",
	"Powerful",
	"Rich",
	"Secret",
	"Smart",
	"Solid",
	"Special",
	"Strategic",
	"Strong",
	"Tidy",
	"Wise",
}

var fruits = []string{
	"Apple",
	"Avocado",
	"Banana",
	"Blackberry",
	"Blueberry",
	"Broccoli",
	"Carrot",
	"Cherry",
	"Coconut",
	"Grape",
	"Lemon",
	"Lettuce",
	"Mango",
	"Melon",
	"Mushroom",
	"Onion",
	"Orange",
	"Papaya",
	"Peach",
	"Pear",
	"Pineapple",
	"Potato",
	"Pumpkin",
	"Raspberry",
	"Strawberry",
	"Tomato",
}

func NameGenerator() string {
	adjective := adjectives[rand.Intn(len(adjectives))]
	fruit := fruits[rand.Intn(len(fruits))]
	return fmt.Sprintf("%s %s", adjective, fruit)
}
