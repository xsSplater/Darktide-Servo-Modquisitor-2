package main

import (
	"testing"
)

func TestParseLocalizationLua(t *testing.T) {
	lua := `
return {
	mod_name = {
		en = "Healthbars",
		ru = "Полоски здоровья",
	},
	["zh-hans"] = {
		name = "敌人血条",
	},
	alm_weapon_selector = { en = "Weapon" },
}`
	parsed := parseLocalizationLua([]byte(lua))

	if parsed["mod_name"]["en"] != "Healthbars" {
		t.Errorf("mod_name.en wrong: %s", parsed["mod_name"]["en"])
	}
	if parsed["mod_name"]["ru"] != "Полоски здоровья" {
		t.Errorf("mod_name.ru wrong: %s", parsed["mod_name"]["ru"])
	}
	if parsed["zh-hans"]["name"] != "敌人血条" {
		t.Errorf("zh-hans.name wrong: %s", parsed["zh-hans"]["name"])
	}
	if parsed["alm_weapon_selector"]["en"] != "Weapon" {
		t.Errorf("alm_weapon_selector.en wrong: %s", parsed["alm_weapon_selector"]["en"])
	}
}
