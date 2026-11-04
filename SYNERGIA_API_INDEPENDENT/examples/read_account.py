"""Manual read-only smoke test. No passwords written to disk."""
import asyncio
from getpass import getpass
from synergia_independent import SynergiaClient


async def main():
    login = input("Login Synergia: ").strip()
    password = getpass("Hasło Synergia (niewidoczne): ")
    async with SynergiaClient(login) as client:
        await client.login(password)
        del password
        info = await client.me()
        print("Zalogowano. Dostępne pola /Me:", sorted(info))
        grades = await client.grades()
        print("Liczba ocen:", len(grades))
        messages = await client.messages(limit=10)
        print("Liczba ostatnich wiadomości:", len(messages))
        days = await client.timetable()
        print("Liczba pozycji planu:", sum(map(len, days.values())))


if __name__ == "__main__":
    asyncio.run(main())
