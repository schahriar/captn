#include "widget.h"

Widget widget_make(const char *label)
{
	Widget w = {label, 3};
	return w;
}

int widget_area(const Widget *w)
{
	return w->width * w->width;
}
