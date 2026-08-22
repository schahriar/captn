#include <string>
#include "gadget.hpp"

int run()
{
	gadgets::Gadget g("card");
	std::string label = g.describe();
	return static_cast<int>(label.size()) + gadgets::rank(g);
}
