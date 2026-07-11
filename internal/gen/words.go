package gen

// Small, fixed word lists for reproducible fake data (no faker library).
var (
	adjectives = []string{
		"Sleek", "Rugged", "Compact", "Premium", "Portable", "Wireless", "Smart",
		"Classic", "Ultra", "Ergonomic", "Durable", "Lightweight", "Advanced", "Eco",
	}
	nouns = []string{
		"Laptop", "Backpack", "Headphones", "Keyboard", "Monitor", "Chair", "Lamp",
		"Speaker", "Camera", "Watch", "Blender", "Bottle", "Jacket", "Sneakers",
	}
	descriptors = []string{
		"built for everyday use", "designed with comfort in mind", "engineered to last",
		"perfect for travel", "loved by reviewers", "a bestseller in its category",
		"backed by a two year warranty", "made from recycled materials",
	}
	categoryNames = []string{
		"Electronics", "Computers", "Audio", "Home & Kitchen", "Fitness", "Outdoors",
		"Fashion", "Footwear", "Furniture", "Office", "Photography", "Wearables",
	}
	cityNames = []string{
		"Jakarta", "Surabaya", "Bandung", "Medan", "Semarang", "Makassar", "Denpasar",
	}
	reviewWords = []string{
		"great", "terrible", "amazing quality", "broke after a week", "works as expected",
		"exceeded my expectations", "would not recommend", "fast shipping", "good value",
		"battery life is poor", "exactly what I needed", "customer service was helpful",
	}
)

// cityCoords gives each city a rough lat/lng so sellers cluster geographically.
var cityCoords = map[string][2]float64{
	"Jakarta":  {-6.2088, 106.8456},
	"Surabaya": {-7.2575, 112.7521},
	"Bandung":  {-6.9175, 107.6191},
	"Medan":    {3.5952, 98.6722},
	"Semarang": {-6.9932, 110.4203},
	"Makassar": {-5.1477, 119.4327},
	"Denpasar": {-8.6500, 115.2167},
}
