<?php

class Widget
{
    private string $label;

    public function __construct(string $label)
    {
        $this->label = $label;
    }

    public function describe(string $prefix): string
    {
        return sprintf('%s: %s', $prefix, $this->label);
    }
}
