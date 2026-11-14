# Generate legacy launcher PNGs (API 24-25) for mipmap-*dpi.
# Adaptive icon (API 26+) lives in res/mipmap-anydpi-v26/ic_launcher.xml.
# Pure ASCII on purpose: Windows PowerShell 5.1 mis-reads UTF-8 without BOM.
Add-Type -AssemblyName System.Drawing

$res = Join-Path (Split-Path $PSScriptRoot -Parent) "app\src\main\res"
$brand = [System.Drawing.ColorTranslator]::FromHtml("#5368D8")
$white = [System.Drawing.Color]::White

$sizes = [ordered]@{ mdpi = 48; hdpi = 72; xhdpi = 96; xxhdpi = 144; xxxhdpi = 192 }

function New-RoundedPath([float]$x, [float]$y, [float]$w, [float]$h, [float]$r) {
    $p = New-Object System.Drawing.Drawing2D.GraphicsPath
    $d = $r * 2
    $p.AddArc($x, $y, $d, $d, 180, 90)
    $p.AddArc($x + $w - $d, $y, $d, $d, 270, 90)
    $p.AddArc($x + $w - $d, $y + $h - $d, $d, $d, 0, 90)
    $p.AddArc($x, $y + $h - $d, $d, $d, 90, 90)
    $p.CloseFigure()
    return $p
}

function New-TrianglePath([float]$x, [float]$y, [float]$w, [float]$h) {
    $p = New-Object System.Drawing.Drawing2D.GraphicsPath
    $p.AddPolygon(@(
        (New-Object System.Drawing.PointF (($x + $w * 0.38), ($y + $h * 0.27))),
        (New-Object System.Drawing.PointF (($x + $w * 0.38), ($y + $h * 0.73))),
        (New-Object System.Drawing.PointF (($x + $w * 0.74), ($y + $h * 0.50)))
    ))
    return $p
}

foreach ($k in $sizes.Keys) {
    $s = [int]$sizes[$k]
    $dir = Join-Path $res ("mipmap-" + $k)
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir | Out-Null }

    foreach ($round in @($false, $true)) {
        $bmp = New-Object System.Drawing.Bitmap($s, $s)
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
        $g.Clear([System.Drawing.Color]::Transparent)

        $fill = New-Object System.Drawing.SolidBrush($brand)
        if ($round) {
            $g.FillEllipse($fill, 0, 0, $s - 1, $s - 1)
        } else {
            $radius = [float]($s * 0.22)
            $rp = New-RoundedPath 0 0 $s $s $radius
            $g.FillPath($fill, $rp)
            $rp.Dispose()
        }

        $tp = New-TrianglePath 0 0 $s $s
        $g.FillPath((New-Object System.Drawing.SolidBrush($white)), $tp)
        $tp.Dispose()
        $fill.Dispose()
        $g.Dispose()

        $name = if ($round) { "ic_launcher_round.png" } else { "ic_launcher.png" }
        $bmp.Save((Join-Path $dir $name), [System.Drawing.Imaging.ImageFormat]::Png)
        $bmp.Dispose()
        Write-Output ("wrote " + $name + " @" + $s)
    }
}
