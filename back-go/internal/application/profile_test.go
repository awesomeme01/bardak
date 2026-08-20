package application

import "testing"

// Обработка полей профиля перед записью — ровно как в Java: имя обрезается по краям,
// пустая или пробельная мордочка становится отсутствующей.
//
// ⚠️ Разница «пустая строка» и «нет значения» видна клиенту: null-поля Jackson вырезает,
// и профиль с avatar = "" отдал бы ключ там, где Java его не отдаёт (MD-003).
func TestTrimmedProfile(t *testing.T) {
	blank := "   "
	empty := ""
	fox := "  🦊  "

	cases := []struct {
		name      string
		display   string
		avatar    *string
		wantName  string
		wantEmoji *string
	}{
		{name: "имя обрезается", display: "  Шабдан  ", avatar: nil, wantName: "Шабдан"},
		{name: "мордочки нет вовсе", display: "Игрок", avatar: nil, wantName: "Игрок"},
		{name: "пустая мордочка стирается", display: "Игрок", avatar: &empty, wantName: "Игрок"},
		{name: "пробельная мордочка стирается", display: "Игрок", avatar: &blank, wantName: "Игрок"},
		{name: "мордочка обрезается", display: "Игрок", avatar: &fox, wantName: "Игрок",
			wantEmoji: pointerTo("🦊")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			name, emoji := trimmedProfile(c.display, c.avatar)
			if name != c.wantName {
				t.Errorf("имя %q, ждали %q", name, c.wantName)
			}
			switch {
			case c.wantEmoji == nil && emoji != nil:
				t.Errorf("мордочка должна была стереться, а осталась %q", *emoji)
			case c.wantEmoji != nil && emoji == nil:
				t.Errorf("мордочка пропала, ждали %q", *c.wantEmoji)
			case c.wantEmoji != nil && *emoji != *c.wantEmoji:
				t.Errorf("мордочка %q, ждали %q", *emoji, *c.wantEmoji)
			}
		})
	}
}

// ⭐ Исходный указатель не переиспользуется: сценарий кладёт в базу обрезанную копию,
// и вызывающий не обнаружит, что его строку молча поменяли под ним.
func TestTrimmedProfileDoesNotTouchCallerValue(t *testing.T) {
	avatar := "  🦊  "

	if _, emoji := trimmedProfile("Игрок", &avatar); emoji == &avatar {
		t.Error("вернулся тот же указатель — правка задела бы значение вызывающего")
	}
	if avatar != "  🦊  " {
		t.Errorf("значение вызывающего изменилось на %q", avatar)
	}
}

func pointerTo(value string) *string { return &value }
