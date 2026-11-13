package com.github.kr328.clash.service.util

import java.nio.file.Files
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test

class RuntimeProfilesTest {
    @Test fun immutableRuntimeAndFailedCandidateSurviveRestart() = runBlocking {
        val root = Files.createTempDirectory("cmfa-runtime").toFile()
        try {
            val source = root.resolve("subscription").apply { mkdir() }
            val config = source.resolve("config.yaml")
            config.writeText("A")
            val store = RuntimeProfiles(root.resolve("runtime"))
            val uuid = UUID.randomUUID()
            val first = store.activate(source, uuid, "profile", null) { assertEquals("A", it.resolve("config.yaml").readText()) }
            config.writeText("invalid B")
            try {
                store.activate(source, UUID.randomUUID(), "broken profile", null) { throw IllegalArgumentException("invalid candidate") }
                fail("invalid candidate accepted")
            } catch (_: IllegalArgumentException) { }
            val restored = RuntimeProfiles(root.resolve("runtime")).current()!!
            assertEquals(first, restored)
            assertEquals("A", restored.directory.resolve("config.yaml").readText())
            config.writeText("renamed C")
            val next = store.activate(source, uuid, "profile", null) { assertEquals("renamed C", it.resolve("config.yaml").readText()) }
            assertEquals(next, RuntimeProfiles(root.resolve("runtime")).current())
            assertEquals("A", first.directory.resolve("config.yaml").readText())
        } finally { root.deleteRecursively() }
    }
}
