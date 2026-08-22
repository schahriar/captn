#include "gadget.hpp"

namespace gadgets {

Gadget::Gadget(std::string label) : label_(label) {}

std::string Gadget::describe() const
{
	return "gadget: " + label_;
}

int rank(const Gadget &g)
{
	return static_cast<int>(g.describe().size());
}

}
