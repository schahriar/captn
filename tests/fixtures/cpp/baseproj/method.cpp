#include <string>

namespace gadgets {

class Widget {
public:
	Widget(int width);
	int area() const;
	std::string describe() const { return label_; }

private:
	std::string label_;
	int width_ = 0;
};

Widget::Widget(int width) : width_(width) {}

int Widget::area() const
{
	return width_ * width_;
}

}
