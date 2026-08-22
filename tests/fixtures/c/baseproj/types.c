#define SCALE(x) ((x) * 2)

typedef int WidgetID;

enum Shape { CIRCLE, SQUARE = 4 };

typedef int (*widget_op)(int);

static int doubler(int v)
{
	return SCALE(v);
}

int apply(widget_op op, WidgetID id)
{
	return op(doubler(id));
}
