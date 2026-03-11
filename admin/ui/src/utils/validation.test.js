import { describe, it, expect } from "vitest";
import { validateInstanceForm } from "./validation.js";

describe("validateInstanceForm", () => {
	it("returns no errors for valid inputs", () => {
		const errors = validateInstanceForm("proxy-1", "10.0.0.1:9090");
		expect(errors.name).toBe("");
		expect(errors.address).toBe("");
	});

	it("returns name error when name is empty", () => {
		const errors = validateInstanceForm("", "10.0.0.1:9090");
		expect(errors.name).toBe("Name is required");
		expect(errors.address).toBe("");
	});

	it("returns address error when address is empty", () => {
		const errors = validateInstanceForm("proxy-1", "");
		expect(errors.address).toBe("Address is required");
		expect(errors.name).toBe("");
	});

	it("returns both errors when both are empty", () => {
		const errors = validateInstanceForm("", "");
		expect(errors.name).toBe("Name is required");
		expect(errors.address).toBe("Address is required");
	});

	it("treats whitespace-only as empty", () => {
		const errors = validateInstanceForm("   ", "  \t ");
		expect(errors.name).toBe("Name is required");
		expect(errors.address).toBe("Address is required");
	});

	it("accepts values with leading/trailing whitespace", () => {
		const errors = validateInstanceForm("  proxy-1  ", "  10.0.0.1:9090  ");
		expect(errors.name).toBe("");
		expect(errors.address).toBe("");
	});
});
