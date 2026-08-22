#ifndef WIDGET_H
#define WIDGET_H

typedef struct Widget {
	const char *label;
	int width;
} Widget;

Widget widget_make(const char *label);
int widget_area(const Widget *w);

#endif
