<?php

interface Renderer
{
    public function render(): string;
}

abstract class Base
{
    public const DEFAULT_LABEL = 'unnamed';
}

class Card extends Base implements Renderer
{
    public function render(): string
    {
        return strtoupper(self::DEFAULT_LABEL);
    }
}

function pick(?Card $card, Base|Renderer $fallback): Card|null
{
    $chosen = $card;
    return $chosen;
}

$make_card = fn(string $label): Card => new Card();
