#include <vector>

namespace gadgets {

struct Base {
	virtual ~Base() = default;
	virtual int weight() const = 0;
};

class Card : public Base {
public:
	int weight() const override { return 2; }
};

using CardList = std::vector<Card>;

enum class Shape { Circle, Square = 4 };

template <typename T>
T last(const CardList &cards, T fallback)
{
	auto scale = [](int v) -> int { return v * 2; };

	if (cards.empty()) {
		return static_cast<T>(scale(static_cast<int>(fallback)));
	}

	return fallback;
}

}
