<?php

namespace App;

class Dep1
{
    public static function exampleText(\DateTime $when): string
    {
        return $when->format('Y');
    }
}
