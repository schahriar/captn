#include <string.h>
#include "widget.h"

int run(void)
{
	Widget w = widget_make("card");
	return (int)strlen(w.label) + widget_area(&w);
}
