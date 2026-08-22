<?php

namespace App;

use DateTime;
use App\Dep1 as FixtureDep1;

require_once 'dep1.php';

function run(): string
{
    $now = new DateTime();
    return FixtureDep1::exampleText($now);
}

echo run();
