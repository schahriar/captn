#ifndef GADGET_HPP
#define GADGET_HPP

#include <string>

namespace gadgets {

class Gadget {
public:
	explicit Gadget(std::string label);
	std::string describe() const;

private:
	std::string label_;
};

int rank(const Gadget &g);

}

#endif
