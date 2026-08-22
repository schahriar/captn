typedef struct Widget {
	const char *label;
	int width;
} Widget;

int widget_area(const Widget *w)
{
	return w->width * w->width;
}
