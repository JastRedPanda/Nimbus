package tray

import "fmt"

func settingsScript(cfg *cfgShim, path string) string {
	units := cfg.Units
	if units == "" {
		units = "celsius"
	}
	pres := cfg.PressureUnit
	if pres == "" {
		pres = "hpa"
	}
	theme := cfg.IconTheme
	if theme == "" {
		theme = "auto"
	}
	lang := cfg.Language
	if lang == "" {
		lang = "en"
	}

	return fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$form = New-Object System.Windows.Forms.Form
$form.Text = "Nimbus Settings"
$form.Size = New-Object System.Drawing.Size(400,420)
$form.StartPosition = "CenterScreen"
$form.FormBorderStyle = "FixedDialog"
$form.MaximizeBox = $false
$form.MinimizeBox = $false

$y = 10

$lbl = New-Object System.Windows.Forms.Label
$lbl.Text = "City:"
$lbl.Location = New-Object System.Drawing.Point(10,$y)
$lbl.Size = New-Object System.Drawing.Size(80,20)
$form.Controls.Add($lbl)

$txtCity = New-Object System.Windows.Forms.TextBox
$txtCity.Location = New-Object System.Drawing.Point(100,$y)
$txtCity.Size = New-Object System.Drawing.Size(180,20)
$txtCity.Text = "%s"
$form.Controls.Add($txtCity)

$y += 30
$lblLat = New-Object System.Windows.Forms.Label
$lblLat.Text = "Latitude:"
$lblLat.Location = New-Object System.Drawing.Point(10,$y)
$lblLat.Size = New-Object System.Drawing.Size(80,20)
$form.Controls.Add($lblLat)

$txtLat = New-Object System.Windows.Forms.TextBox
$txtLat.Location = New-Object System.Drawing.Point(100,$y)
$txtLat.Size = New-Object System.Drawing.Size(80,20)
$txtLat.Text = "%.4f"
$form.Controls.Add($txtLat)

$y += 30
$lblLon = New-Object System.Windows.Forms.Label
$lblLon.Text = "Longitude:"
$lblLon.Location = New-Object System.Drawing.Point(10,$y)
$lblLon.Size = New-Object System.Drawing.Size(80,20)
$form.Controls.Add($lblLon)

$txtLon = New-Object System.Windows.Forms.TextBox
$txtLon.Location = New-Object System.Drawing.Point(100,$y)
$txtLon.Size = New-Object System.Drawing.Size(80,20)
$txtLon.Text = "%.4f"
$form.Controls.Add($txtLon)

$y += 40
$grpT = New-Object System.Windows.Forms.GroupBox
$grpT.Text = "Temperature"
$grpT.Location = New-Object System.Drawing.Point(10,$y)
$grpT.Size = New-Object System.Drawing.Size(180,50)
$form.Controls.Add($grpT)

$rbC = New-Object System.Windows.Forms.RadioButton
$rbC.Text = "°C"
$rbC.Location = New-Object System.Drawing.Point(10,20)
$rbC.Size = New-Object System.Drawing.Size(60,20)
if ("%s" -eq "celsius") { $rbC.Checked = $true }
$grpT.Controls.Add($rbC)

$rbF = New-Object System.Windows.Forms.RadioButton
$rbF.Text = "°F"
$rbF.Location = New-Object System.Drawing.Point(90,20)
$rbF.Size = New-Object System.Drawing.Size(60,20)
if ("%s" -eq "fahrenheit") { $rbF.Checked = $true }
$grpT.Controls.Add($rbF)

$y += 60
$grpP = New-Object System.Windows.Forms.GroupBox
$grpP.Text = "Pressure"
$grpP.Location = New-Object System.Drawing.Point(10,$y)
$grpP.Size = New-Object System.Drawing.Size(280,50)
$form.Controls.Add($grpP)

$rbH = New-Object System.Windows.Forms.RadioButton
$rbH.Text = "hPa"
$rbH.Location = New-Object System.Drawing.Point(10,20)
$rbH.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "hpa") { $rbH.Checked = $true }
$grpP.Controls.Add($rbH)

$rbM = New-Object System.Windows.Forms.RadioButton
$rbM.Text = "mmHg"
$rbM.Location = New-Object System.Drawing.Point(90,20)
$rbM.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "mmhg") { $rbM.Checked = $true }
$grpP.Controls.Add($rbM)

$rbI = New-Object System.Windows.Forms.RadioButton
$rbI.Text = "inHg"
$rbI.Location = New-Object System.Drawing.Point(170,20)
$rbI.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "inhg") { $rbI.Checked = $true }
$grpP.Controls.Add($rbI)

$y += 60
$grpTh = New-Object System.Windows.Forms.GroupBox
$grpTh.Text = "Icon Theme"
$grpTh.Location = New-Object System.Drawing.Point(10,$y)
$grpTh.Size = New-Object System.Drawing.Size(280,50)
$form.Controls.Add($grpTh)

$thA = New-Object System.Windows.Forms.RadioButton
$thA.Text = "Auto"
$thA.Location = New-Object System.Drawing.Point(10,20)
$thA.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "auto") { $thA.Checked = $true }
$grpTh.Controls.Add($thA)

$thD = New-Object System.Windows.Forms.RadioButton
$thD.Text = "Dark"
$thD.Location = New-Object System.Drawing.Point(90,20)
$thD.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "dark") { $thD.Checked = $true }
$grpTh.Controls.Add($thD)

$thL = New-Object System.Windows.Forms.RadioButton
$thL.Text = "Light"
$thL.Location = New-Object System.Drawing.Point(170,20)
$thL.Size = New-Object System.Drawing.Size(70,20)
if ("%s" -eq "light") { $thL.Checked = $true }
$grpTh.Controls.Add($thL)

$y += 60
$grpL = New-Object System.Windows.Forms.GroupBox
$grpL.Text = "Language"
$grpL.Location = New-Object System.Drawing.Point(10,$y)
$grpL.Size = New-Object System.Drawing.Size(180,50)
$form.Controls.Add($grpL)

$rbEn = New-Object System.Windows.Forms.RadioButton
$rbEn.Text = "English"
$rbEn.Location = New-Object System.Drawing.Point(10,20)
$rbEn.Size = New-Object System.Drawing.Size(80,20)
if ("%s" -eq "en") { $rbEn.Checked = $true }
$grpL.Controls.Add($rbEn)

$rbUk = New-Object System.Windows.Forms.RadioButton
$rbUk.Text = "Українська"
$rbUk.Location = New-Object System.Drawing.Point(100,20)
$rbUk.Size = New-Object System.Drawing.Size(80,20)
if ("%s" -eq "uk") { $rbUk.Checked = $true }
$grpL.Controls.Add($rbUk)

$y += 70
$btnOk = New-Object System.Windows.Forms.Button
$btnOk.Text = "Save"
$btnOk.Location = New-Object System.Drawing.Point(100,$y)
$btnOk.Size = New-Object System.Drawing.Size(80,30)
$btnOk.DialogResult = "OK"
$form.Controls.Add($btnOk)

$btnCancel = New-Object System.Windows.Forms.Button
$btnCancel.Text = "Cancel"
$btnCancel.Location = New-Object System.Drawing.Point(200,$y)
$btnCancel.Size = New-Object System.Drawing.Size(80,30)
$btnCancel.DialogResult = "Cancel"
$form.Controls.Add($btnCancel)

$result = $form.ShowDialog()
if ($result -eq "OK") {
    $unit = "celsius"
    if ($rbF.Checked) { $unit = "fahrenheit" }
    $pres = "hpa"
    if ($rbM.Checked) { $pres = "mmhg" }
    if ($rbI.Checked) { $pres = "inhg" }
    $th = "auto"
    if ($thD.Checked) { $th = "dark" }
    if ($thL.Checked) { $th = "light" }
    $lang = "en"
    if ($rbUk.Checked) { $lang = "uk" }
    $lat = [double]::Parse($txtLat.Text)
    $lon = [double]::Parse($txtLon.Text)
    $city = $txtCity.Text
    @"
{"latitude":$lat,"longitude":$lon,"city_name":"$city","update_interval":10,"units":"$unit","pressure_unit":"$pres","icon_theme":"$th","language":"$lang"}
"@
}
`, cfg.CityName, cfg.Latitude, cfg.Longitude, units, units, pres, pres, pres, theme, theme, theme, lang, lang)
}

type cfgShim struct {
	CityName     string
	Latitude     float64
	Longitude    float64
	Units        string
	PressureUnit string
	IconTheme    string
	Language     string
}
