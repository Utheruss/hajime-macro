package main

import (
	"fmt"
	"strings"
)

type Lang int

const (
	LangRU Lang = iota
	LangEN
	LangES
)

var langNames = []string{"Русский", "English", "Español"}

// tr: ключ -> {русский, английский, испанский}
var tr = map[string][3]string{
	// ----- окно -----
	"ui.what":       {"Что качаем:", "What to train:", "Qué entrenar:"},
	"ui.start":      {"Старт (F6)", "Start (F6)", "Iniciar (F6)"},
	"ui.stop":       {"Стоп (F6)", "Stop (F6)", "Detener (F6)"},
	"ui.stamina":    {"Стамина: %d%%", "Stamina: %d%%", "Aguante: %d%%"},
	"ui.stam_none":  {"Стамина: не вижу", "Stamina: not visible", "Aguante: no se ve"},
	"ui.session":    {"Тренировок: %d  ·  Ударов: %d  ·  Время: %s", "Trainings: %d  ·  Punches: %d  ·  Time: %s", "Entrenos: %d  ·  Golpes: %d  ·  Tiempo: %s"},
	"ui.lead":       {"Опережение, мс:", "Lead time, ms:", "Adelanto, ms:"},
	"ui.lead_hint":  {"клики поздно - больше", "clicks late? raise it", "si va tarde, súbelo"},
	"ui.punch":      {"Пауза между ударами, мс:", "Delay between punches, ms:", "Pausa entre golpes, ms:"},
	"ui.punch_hint": {"(для груши)", "(for the bag)", "(para el saco)"},
	"ui.diag":       {"Диагностика", "Diagnostics", "Diagnóstico"},
	"ui.hotkey":     {"F6 - старт/стоп из игры", "F6 - start/stop in game", "F6 - iniciar/parar en juego"},
	"ui.guide_tip":  {"Руководство", "User guide", "Guía de uso"},

	// ----- статы -----
	"stat.arm":    {"Arm strength - руки", "Arm strength (arms)", "Arm strength (brazos)"},
	"stat.leg":    {"Leg strength - ноги", "Leg strength (legs)", "Leg strength (piernas)"},
	"stat.torso":  {"Torso endurance - торс", "Torso endurance (torso)", "Torso endurance (torso)"},
	"stat.pspeed": {"Punch speed - скорость удара", "Punch speed", "Punch speed (velocidad)"},
	"stat.jump":   {"Jump rope - скакалка", "Jump rope", "Jump rope (comba)"},
	"stat.sharp":  {"Punch sharpness - резкость удара", "Punch sharpness (bag)", "Punch sharpness (saco)"},

	"hint.bar":    {"Полоска: режим (клик или зажатие) определяется сам.", "Timing bar: the mode (click or hold) is detected automatically.", "Barra de tiempo: el modo (clic o mantener) se detecta solo."},
	"hint.pspeed": {"Полоска. Сначала надень перчатки (клавиша 1).", "Timing bar. Equip the gloves first (key 1).", "Barra de tiempo. Equipa antes los guantes (tecla 1)."},
	"hint.sharp":  {"Груша: бьёт ЛКМ и ПКМ по очереди. Перчатки (1) надень заранее.", "Bag: hits with left and right click in turns. Equip gloves (1) first.", "Saco: golpea con clic izquierdo y derecho por turnos. Equipa los guantes (1)."},

	// ----- статус -----
	"st.ready":       {"Нажми F6 у тренажёра", "Press F6 at the equipment", "Pulsa F6 junto al equipo"},
	"st.idle":        {"Остановлено. F6 - старт у тренажёра", "Stopped. F6 - start at the equipment", "Detenido. F6 - iniciar junto al equipo"},
	"st.stopped":     {"Остановлено - управление у тебя. F6 - запустить снова", "Stopped - you have control. F6 to start again", "Detenido: tú tienes el control. F6 para iniciar de nuevo"},
	"st.starting":    {"Запуск...", "Starting...", "Iniciando..."},
	"st.nowin":       {"Не вижу окно Roblox", "Can't see the Roblox window", "No veo la ventana de Roblox"},
	"st.nostam":      {"Не вижу шкалу стамины. Игра на экране?", "Can't see the stamina bar. Is the game on screen?", "No veo la barra de aguante. ¿El juego está en pantalla?"},
	"st.wait_stam":   {"Жду стамину %d%%", "Waiting for stamina %d%%", "Esperando aguante %d%%"},
	"st.full_wait":   {"Стамина 100%, жду 1 секунду", "Stamina 100%, waiting 1 second", "Aguante al 100%, espero 1 segundo"},
	"st.pressed_e":   {"Нажал E, жду полоску", "Pressed E, waiting for the bar", "Pulsé E, esperando la barra"},
	"st.cant_start":  {"Не запускается. Встань у тренажёра (подсказка E) и жми F6", "Won't start. Stand at the equipment (E prompt) and press F6", "No arranca. Ponte junto al equipo (aviso de E) y pulsa F6"},
	"st.training":    {"Тренируется (режим %d)", "Training (mode %d)", "Entrenando (modo %d)"},
	"st.ended":       {"Тренировка кончилась. Жду стамину", "Training ended. Waiting for stamina", "Entreno terminado. Esperando aguante"},
	"st.bag":         {"Бью грушу", "Hitting the bag", "Golpeando el saco"},
	"st.bag_rest":    {"Стамина кончилась, жду восстановления", "Out of stamina, waiting to recover", "Sin aguante, esperando a recuperar"},
	"st.nogloves":    {"Ожидание: перчатки не надеты", "Waiting: gloves not equipped", "Esperando: guantes no equipados"},
	"warn.gloves":    {"Наденьте перчатки (клавиша 1)", "Equip your gloves (press 1)", "Equipa los guantes (tecla 1)"},
	"warn.cap":       {"Достигнут кап: тренировка остановлена", "Stat cap reached: training stopped", "Límite alcanzado: entrenamiento detenido"},
	"warn.supply":    {"Автопополнение не помогло: проверьте предметы", "Auto-supply failed: check your items", "El auto-suministro falló: revisa tus objetos"},
	"warn.energy":    {"Энергия ниже 10%: восстановите", "Energy below 10%: restore it", "Energía bajo 10 %: recupérala"},
	"ui.auto_e":      {"Авто-энергия", "Auto energy", "Energía auto"},
	"ui.auto_f":      {"Авто-еда", "Auto food", "Comida auto"},
	"ui.auto_w":      {"Авто-вода", "Auto water", "Agua auto"},
	"st.cap_stop":    {"Кап достигнут: тренировка остановлена", "Cap reached: training stopped", "Límite alcanzado: entrenamiento detenido"},
	"st.supply_stop": {"Останавливаю тренировку (E)...", "Stopping the training (E)...", "Deteniendo el entrenamiento (E)..."},
	"st.supply_e":    {"Авто-энергия: использую предмет (клавиша 4)", "Auto energy: using the item (key 4)", "Energía auto: usando el objeto (tecla 4)"},
	"st.supply_f":    {"Авто-еда: ем (клавиша 2)", "Auto food: eating (key 2)", "Comida auto: comiendo (tecla 2)"},
	"st.supply_w":    {"Авто-вода: пью (клавиша 3)", "Auto water: drinking (key 3)", "Agua auto: bebiendo (tecla 3)"},
	"ui.warning":     {"ВНИМАНИЕ", "WARNING", "ATENCIÓN"},
	"st.paused":      {"Пауза: окно игры не активно", "Paused: the game window is not active", "Pausa: la ventana del juego no está activa"},
	"st.hotkey_busy": {"F6 занята другой программой. Используй кнопку Старт", "F6 is used by another program. Use the Start button", "F6 la usa otro programa. Usa el botón Iniciar"},
	"st.diag_saved":  {"Диагностика сохранена рядом с exe", "Diagnostics saved next to the exe", "Diagnóstico guardado junto al exe"},
	"st.error":       {"Ошибка: %v", "Error: %v", "Error: %v"},

	// ----- руководство -----
	"guide.title": {"Hajime Macro - руководство", "Hajime Macro - user guide", "Hajime Macro - guía de uso"},
	"guide.text":  {guideRU, guideEN, guideES},
}

// T возвращает текст на нужном языке; args подставляются как в fmt.Sprintf.
func T(l Lang, key string, args ...any) string {
	v, ok := tr[key]
	if !ok {
		return key
	}
	s := v[int(l)]
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	// "%%" без аргументов остаётся как обычный процент
	return strings.ReplaceAll(s, "%%", "%")
}

const guideEN = `HAJIME MACRO - USER GUIDE

WHAT IT DOES
Automates the stat-training mini-games of Hajime Rising. It reads the screen (like a screenshot) and presses the mouse buttons and E for you. It does not touch the game's memory or files.

QUICK START
1. Run HajimeMacro.exe. Play Roblox in windowed or borderless mode (not exclusive fullscreen).
2. Pick what you are training in the list.
3. Stand at the equipment, where the "E - Start" prompt is visible.
4. Press F6 (it works from inside the game). Press F6 again to stop: the macro presses E to end the training and gives control back to you.

WHAT THE MACRO DOES
- Waits until stamina is 100%, waits 1 more second, then presses E to start training. When stamina runs out it waits again and restarts.
- Timing bar, "click" type (jump rope, punch speed): clicks every time the arrow crosses the middle.
- Timing bar, "hold" type (arms, legs, torso): holds the left button when the arrow enters the white bar and releases it when the arrow leaves. The type is detected automatically from the bar on screen.
- Punch sharpness (bag): hits the bag with the left and right mouse buttons in turns. When stamina runs out it waits for full recovery and continues. It does not press E.
- Punch speed and punch sharpness need boxing gloves: press 1 in the game first. For the bag the macro warns you (beep and a message) if the gloves are not equipped.
- If the Roblox window is not the active window, the macro pauses by itself and continues when you come back to the game.

WARNINGS
Red "WARNING" appears at the bottom of the window together with a beep. Several warnings can be shown at once.
- Stat cap reached: the game says "... is at your goal ... training won't raise it further". The macro beeps, stops the training (like F6, with E) and does not start it again.
- Energy below 10%: the yellow energy bar at the bottom right is almost empty. Restore it. The check is repeated at the start of every training.
- Gloves not equipped (Punch speed, Punch sharpness): the macro waits until you equip them (key 1).

AUTO ENERGY / FOOD / WATER
Three check boxes under the Start button. When one is on, the macro watches the three bars at the bottom right (yellow energy, food, water).
- It acts when energy drops below 10%, or food or water is at 5% or less. The macro stops the training with E and then:
  - energy: key 4, 0.25 s, left click, 0.25 s, left click;
  - food: key 2, 0.25 s, four left clicks 0.25 s apart;
  - water: key 3, 0.25 s, two left clicks 0.25 s apart.
- Then it waits for full stamina, waits 1 more second and starts training again. For Punch speed and Punch sharpness it puts the gloves back on (key 1) first.
- Put the energy item in slot 4, food in slot 2 and water in slot 3 of your hotbar.
- If a bar is empty again right after using the item (no items left?), the macro does not retry, shows a warning and goes on training.
- For energy you also get the beep and the warning. For food and water there is no sound.
- Untick a box to turn that feature off.

SETTINGS
- Lead time, ms: compensates screen and input delay. Clicks too late? Raise it (50, 70...). Too early? Lower it. It matters most on fast equipment.
- Delay between punches, ms: the pause between bag hits (300 by default).
- Language: the box in the top right corner (Russian, English, Spanish).
- The macro remembers your choice, settings and window position.

SCREEN SIZE
It works with different resolutions and interface sizes: it finds the bars on the screen by itself.

IF SOMETHING DOESN'T WORK
- "Can't see the stamina bar": the game is minimized, covered, or in exclusive fullscreen. Use windowed or borderless mode (F11 in Roblox switches fullscreen).
- F6 does nothing: another program uses F6. Use the Start button.
- Click "Diagnostics": the macro saves hajime_diag.png and hajime_diag.txt next to the exe. Send them to the developer.
- Antivirus warning: the macro imitates the mouse and keyboard and reads the screen, and some antiviruses dislike that (false positive). The file is not digitally signed.

WARNING
Automation can go against a game's rules. Use it at your own risk.`

const guideRU = `HAJIME MACRO - РУКОВОДСТВО

ЧТО ЭТО
Автоматизирует мини-игры прокачки статов в Hajime Rising. Читает экран (как скриншот) и сам нажимает кнопки мыши и E. В память и файлы игры не лезет.

БЫСТРЫЙ СТАРТ
1. Запусти HajimeMacro.exe. Играй в Roblox в оконном или borderless-режиме (не в эксклюзивном полноэкранном).
2. Выбери в списке, что качаешь.
3. Встань у тренажёра, где видна подсказка "E - Start".
4. Нажми F6 (работает прямо из игры). Повторное F6 останавливает: макрос нажмёт E, чтобы прервать тренировку, и отдаст управление тебе.

ЧТО ДЕЛАЕТ МАКРОС
- Ждёт, пока стамина станет 100%, ждёт ещё 1 секунду и нажимает E, чтобы начать тренировку. Когда стамина кончилась, снова ждёт и перезапускает.
- Полоска, тип "клик" (скакалка, скорость удара): кликает каждый раз, когда стрелка пересекает середину.
- Полоска, тип "зажатие" (руки, ноги, торс): зажимает левую кнопку, когда стрелка входит в белый бар, и отпускает, когда выходит. Тип определяется сам по полоске на экране.
- Punch sharpness (груша): бьёт грушу левой и правой кнопкой мыши по очереди. Когда стамина кончилась, ждёт полного восстановления и продолжает. E не нажимает.
- Для punch speed и punch sharpness нужны боксёрские перчатки: сначала нажми 1 в игре. На груше макрос предупредит (писк и сообщение), если перчатки не надеты.
- Если окно Roblox не активно, макрос сам встаёт на паузу и продолжает, когда ты вернёшься в игру.

ПРЕДУПРЕЖДЕНИЯ
Внизу окна появляется красное "ВНИМАНИЕ" и звучит сигнал. Несколько предупреждений могут показываться одновременно.
- Достигнут кап стата: игра пишет "... is at your goal ... training won't raise it further". Макрос подаёт сигнал, прерывает тренировку (как F6, клавишей E) и больше её не начинает.
- Энергии меньше 10%: жёлтая шкала энергии справа внизу почти пуста. Восстановите её. Проверка повторяется в начале каждой тренировки.
- Перчатки не надеты (Punch speed, Punch sharpness): макрос ждёт, пока вы наденете их (клавиша 1).

АВТО-ЭНЕРГИЯ / ЕДА / ВОДА
Три галочки под кнопкой Старт. Если галочка включена, макрос следит за тремя шкалами справа внизу (жёлтая - энергия, еда, вода).
- Он срабатывает, когда энергии меньше 10% либо еды или воды 5% и меньше. Макрос прерывает тренировку клавишей E и затем:
  - энергия: клавиша 4, 0.25 с, ЛКМ, 0.25 с, ЛКМ;
  - еда: клавиша 2, 0.25 с, четыре ЛКМ с паузой 0.25 с;
  - вода: клавиша 3, 0.25 с, два ЛКМ с паузой 0.25 с.
- Потом ждёт полную стамину, ещё 1 секунду и снова начинает тренировку. Для Punch speed и Punch sharpness перед этим снова надевает перчатки (клавиша 1).
- Положите предмет с энергией в слот 4, еду в слот 2, воду в слот 3 панели.
- Если шкала снова пуста сразу после использования (кончились предметы?), макрос не повторяет попытку, показывает предупреждение и продолжает тренироваться.
- Для энергии есть ещё сигнал и предупреждение. Для еды и воды звука нет.
- Снимите галочку, чтобы отключить функцию.

НАСТРОЙКИ
- Опережение, мс: компенсирует задержку экрана и ввода. Клики поздно? Увеличь (50, 70...). Рано? Уменьши. Важнее всего на быстрых тренажёрах.
- Пауза между ударами, мс: пауза между ударами по груше (по умолчанию 300).
- Язык: список в правом верхнем углу (русский, английский, испанский).
- Макрос запоминает выбор, настройки и положение окна.

РАЗМЕР ЭКРАНА
Работает с разными разрешениями и размерами интерфейса: полоски на экране он находит сам.

ЕСЛИ ЧТО-ТО НЕ РАБОТАЕТ
- "Не вижу шкалу стамины": игра свёрнута, закрыта другим окном или в эксклюзивном полноэкранном режиме. Используй оконный или borderless-режим (F11 в Roblox переключает полный экран).
- F6 не реагирует: её занимает другая программа. Используй кнопку Старт.
- Нажми "Диагностика": рядом с exe появятся hajime_diag.png и hajime_diag.txt. Пришли их разработчику.
- Антивирус ругается: макрос имитирует мышь и клавиатуру и читает экран, некоторым антивирусам это не нравится (ложное срабатывание). Файл не имеет цифровой подписи.

ВНИМАНИЕ
Автоматизация может противоречить правилам игры. Используй на свой риск.`

const guideES = `HAJIME MACRO - GUÍA DE USO

QUÉ HACE
Automatiza los minijuegos de entrenamiento de estadísticas de Hajime Rising. Lee la pantalla (como una captura) y pulsa por ti los botones del ratón y la tecla E. No toca la memoria ni los archivos del juego.

INICIO RÁPIDO
1. Ejecuta HajimeMacro.exe. Juega a Roblox en modo ventana o sin bordes (no en pantalla completa exclusiva).
2. Elige en la lista qué vas a entrenar.
3. Colócate junto al equipo, donde se ve el aviso "E - Start".
4. Pulsa F6 (funciona desde dentro del juego). Vuelve a pulsar F6 para parar: el macro pulsa E para terminar el entrenamiento y te devuelve el control.

QUÉ HACE EL MACRO
- Espera a que el aguante (stamina) llegue al 100 %, espera 1 segundo más y pulsa E para empezar. Cuando el aguante se acaba, vuelve a esperar y reinicia.
- Barra de tiempo, tipo "clic" (jump rope, punch speed): hace clic cada vez que la flecha cruza el centro.
- Barra de tiempo, tipo "mantener" (brazos, piernas, torso): mantiene el botón izquierdo cuando la flecha entra en la barra blanca y lo suelta cuando sale. El tipo se detecta solo a partir de la barra en pantalla.
- Punch sharpness (saco): golpea el saco con los botones izquierdo y derecho del ratón por turnos. Cuando se acaba el aguante, espera a recuperarlo por completo y sigue. No pulsa E.
- Punch speed y punch sharpness necesitan guantes de boxeo: pulsa antes 1 en el juego. En el saco el macro te avisa (pitido y mensaje) si no llevas los guantes.
- Si la ventana de Roblox no está activa, el macro se pone en pausa solo y continúa cuando vuelves al juego.

AVISOS
Abajo en la ventana aparece "ATENCIÓN" en rojo y suena un pitido. Pueden mostrarse varios avisos a la vez.
- Límite del stat alcanzado: el juego dice "... is at your goal ... training won't raise it further". El macro pita, detiene el entrenamiento (como F6, con E) y no lo vuelve a iniciar.
- Energía por debajo del 10 %: la barra amarilla de energía (abajo a la derecha) está casi vacía. Recupérala. Se comprueba al inicio de cada entrenamiento.
- Guantes no equipados (Punch speed, Punch sharpness): el macro espera hasta que te los equipes (tecla 1).

ENERGÍA / COMIDA / AGUA AUTO
Tres casillas bajo el botón Iniciar. Si una está activa, el macro vigila las tres barras de abajo a la derecha (amarilla: energía, comida, agua).
- Actúa cuando la energía baja del 10 %, o la comida o el agua están al 5 % o menos. El macro detiene el entrenamiento con E y luego:
  - energía: tecla 4, 0,25 s, clic izquierdo, 0,25 s, clic izquierdo;
  - comida: tecla 2, 0,25 s, cuatro clics izquierdos con 0,25 s entre ellos;
  - agua: tecla 3, 0,25 s, dos clics izquierdos con 0,25 s entre ellos.
- Después espera la resistencia completa, 1 segundo más y vuelve a entrenar. En Punch speed y Punch sharpness antes vuelve a poner los guantes (tecla 1).
- Pon el objeto de energía en la ranura 4, la comida en la 2 y el agua en la 3 de la barra.
- Si la barra vuelve a estar vacía justo después de usar el objeto (¿sin objetos?), el macro no reintenta, muestra un aviso y sigue entrenando.
- Con la energía también hay pitido y aviso. Con comida y agua no hay sonido.
- Desmarca la casilla para desactivar esa función.

AJUSTES
- Adelanto, ms: compensa el retraso de la pantalla y de la entrada. ¿Los clics llegan tarde? Súbelo (50, 70...). ¿Demasiado pronto? Bájalo. Importa sobre todo en equipos rápidos.
- Pausa entre golpes, ms: la pausa entre golpes al saco (300 por defecto).
- Idioma: la lista de la esquina superior derecha (ruso, inglés, español).
- El macro recuerda tu elección, los ajustes y la posición de la ventana.

TAMAÑO DE PANTALLA
Funciona con distintas resoluciones y tamaños de interfaz: encuentra las barras en pantalla por sí solo.

SI ALGO NO FUNCIONA
- "No veo la barra de aguante": el juego está minimizado, tapado por otra ventana o en pantalla completa exclusiva. Usa el modo ventana o sin bordes (F11 en Roblox cambia la pantalla completa).
- F6 no responde: otro programa la usa. Usa el botón Iniciar.
- Pulsa "Diagnóstico": junto al exe se guardan hajime_diag.png y hajime_diag.txt. Envíaselos al desarrollador.
- Aviso del antivirus: el macro imita el ratón y el teclado y lee la pantalla, y a algunos antivirus eso no les gusta (falso positivo). El archivo no tiene firma digital.

AVISO
La automatización puede ir contra las reglas de un juego. Úsalo bajo tu propia responsabilidad.`
