package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ANSI color codes for better terminal output (auto-disable if NO_COLOR or not TTY-like)
var (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

func init() {
	if !colorsEnabled() {
		ColorReset = ""
		ColorRed = ""
		ColorGreen = ""
		ColorYellow = ""
		ColorBlue = ""
		ColorPurple = ""
		ColorCyan = ""
		ColorBold = ""
	}
}

func colorsEnabled() bool {
	// Disable if NO_COLOR or TERM=dumb
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}
	// Heuristic: if stdout isn't a char device, disable (non-TTY)
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

type Word struct {
	Word       string `json:"word"`
	Article    string `json:"article"`
	English    string `json:"english,omitempty"`
	Category   string `json:"category,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Plural     string `json:"plural,omitempty"`
}

type Words struct {
	Version string `json:"version"`
	Data    []Word `json:"data"`
}

type Stats struct {
	TotalQuizzes   int            `json:"total_quizzes"`
	TotalQuestions int            `json:"total_questions"`
	CorrectAnswers int            `json:"correct_answers"`
	WordStats      map[string]int `json:"word_stats"` // lifetime mistakes per word
}

type MistakeInfo struct {
	word          Word
	userAnswer    string
	correctAnswer string
}

type Quiz struct {
	words  []Word
	stats  *Stats
	reader *bufio.Reader
	rng    *rand.Rand
	sessionStats struct {
		correct   int
		total     int
		mistakes  []MistakeInfo
		startTime time.Time
	}
}

func main() {
	quiz := NewQuiz()

	quiz.setupSignalHandler()

	if err := quiz.LoadWords("words.json"); err != nil {
		fmt.Printf("%sError loading words: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	quiz.LoadStats()
	defer quiz.SaveStats()

	quiz.ShowWelcome()
	quiz.RunGameLoop()
}

func NewQuiz() *Quiz {
	return &Quiz{
		reader: bufio.NewReader(os.Stdin),
		stats:  &Stats{WordStats: make(map[string]int)},
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (q *Quiz) setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println()
		fmt.Printf("%sSaving stats and exiting...%s\n", ColorYellow, ColorReset)
		q.SaveStats()
		os.Exit(0)
	}()
}

func (q *Quiz) LoadWords(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		// Try a couple of fallback locations
		alt := filepath.Join(getDataDir(), filename)
		if f2, err2 := os.Open(alt); err2 == nil {
			defer f2.Close()
			var words Words
			if err := json.NewDecoder(f2).Decode(&words); err != nil {
				return fmt.Errorf("could not decode JSON at %s: %w", alt, err)
			}
			if len(words.Data) == 0 {
				return fmt.Errorf("no words found in %s", alt)
			}
			q.words = words.Data
			return nil
		}
		return fmt.Errorf("could not open %s: %w", filename, err)
	}
	defer file.Close()

	var words Words
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&words); err != nil {
		return fmt.Errorf("could not decode JSON: %w", err)
	}

	if len(words.Data) == 0 {
		return fmt.Errorf("no words found in file")
	}

	q.words = words.Data
	return nil
}

func (q *Quiz) LoadStats() {
	statsFile := filepath.Join(getDataDir(), "stats.json")
	file, err := os.Open(statsFile)
	if err != nil {
		q.ensureStatsDefaults()
		return
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	if err := dec.Decode(q.stats); err != nil {
		fmt.Printf("%sWarning: could not parse stats.json, starting fresh (%v)%s\n", ColorYellow, err, ColorReset)
		q.stats = &Stats{}
	}
	q.ensureStatsDefaults()
}

func (q *Quiz) ensureStatsDefaults() {
	if q.stats == nil {
		q.stats = &Stats{}
	}
	if q.stats.WordStats == nil {
		q.stats.WordStats = make(map[string]int)
	}
}

func (q *Quiz) SaveStats() {
	dataDir := getDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create data dir %s: %v\n", dataDir, err)
		return
	}

	statsFile := filepath.Join(dataDir, "stats.json")
	file, err := os.Create(statsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save stats: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(q.stats); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write stats: %v\n", err)
	}
}

func getDataDir() string {
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		return filepath.Join(cfg, "german-quiz")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".german_quiz")
}

func (q *Quiz) ShowWelcome() {
	fmt.Printf("%s%s=== German Article Quiz ===%s\n\n", ColorBold, ColorBlue, ColorReset)

	if q.stats.TotalQuestions > 0 {
		accuracy := float64(q.stats.CorrectAnswers) / float64(q.stats.TotalQuestions) * 100
		fmt.Printf("Welcome back! Your overall accuracy: %s%.1f%%%s\n", ColorCyan, accuracy, ColorReset)
		fmt.Printf("Total quizzes completed: %d\n\n", q.stats.TotalQuizzes)
	} else {
		fmt.Printf("Welcome! Let’s start with a quick quiz to build your stats.\n\n")
	}
}

func (q *Quiz) RunGameLoop() {
	for {
		q.ShowMenu()
		choice := q.getInput()

		switch strings.ToLower(choice) {
		case "1":
			q.StartQuiz(10, "")
		case "2":
			q.ShowCustomMenu()
		case "3":
			q.ShowDetailedStats()
		case "4":
			q.ShowPracticeMode()
		case "q", "quit", "exit":
			fmt.Printf("\n%sTschüss! Keep practicing!%s\n", ColorYellow, ColorReset)
			return
		default:
			fmt.Printf("%sInvalid choice. Please try again.%s\n", ColorRed, ColorReset)
		}
	}
}

func (q *Quiz) ShowMenu() {
	fmt.Printf("\n%sMain Menu:%s\n", ColorBold, ColorReset)
	fmt.Println("1. Quick Quiz (10 questions)")
	fmt.Println("2. Custom Quiz")
	fmt.Println("3. View Statistics")
	fmt.Println("4. Practice Mode (focus on mistakes)")
	fmt.Println("q. Quit")
	fmt.Print("\nYour choice: ")
}

func (q *Quiz) ShowCustomMenu() {
	fmt.Print("\nHow many questions? (5-50): ")
	numStr := q.getInput()
	num, err := strconv.Atoi(numStr)
	if err != nil || num < 5 || num > 50 {
		fmt.Printf("%sInvalid number. Using default of 10.%s\n", ColorRed, ColorReset)
		num = 10
	}

	fmt.Println("\nSelect difficulty:")
	fmt.Println("1. All levels")
	fmt.Println("2. Easy only")
	fmt.Println("3. Medium only")
	fmt.Println("4. Hard only")
	fmt.Print("Your choice: ")

	difficulty := ""
	switch strings.ToLower(q.getInput()) {
	case "2", "easy", "e":
		difficulty = "easy"
	case "3", "medium", "m":
		difficulty = "medium"
	case "4", "hard", "h":
		difficulty = "hard"
	}

	q.StartQuiz(num, difficulty)
}

func (q *Quiz) StartQuiz(numQuestions int, difficulty string) {
	// Filter words by difficulty if specified (strict)
	availableWords := q.words
	if difficulty != "" {
		filtered := make([]Word, 0, len(q.words))
		for _, w := range q.words {
			if strings.EqualFold(w.Difficulty, difficulty) {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			availableWords = filtered
		} else {
			fmt.Printf("%sNo words found for '%s'. Using all levels.%s\n", ColorYellow, difficulty, ColorReset)
		}
	}

	if len(availableWords) == 0 {
		fmt.Printf("%sNo words available to quiz.%s\n", ColorRed, ColorReset)
		return
	}

	if numQuestions > len(availableWords) {
		numQuestions = len(availableWords)
	}

	// Shuffle words
	shuffled := make([]Word, len(availableWords))
	copy(shuffled, availableWords)
	q.rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Reset session stats
	q.sessionStats.correct = 0
	q.sessionStats.total = numQuestions
	q.sessionStats.mistakes = []MistakeInfo{}
	q.sessionStats.startTime = time.Now()

	fmt.Printf("\n%s%sStarting quiz with %d questions...%s\n", ColorBold, ColorCyan, numQuestions, ColorReset)
	fmt.Println(strings.Repeat("-", 40))

	answered := 0
	for i := 0; i < numQuestions; i++ {
		if cont := q.askQuestion(shuffled[i], i+1, numQuestions); !cont {
			// Early exit; count only answered so far
			q.sessionStats.total = answered
			break
		}
		answered++
	}

	q.showResults()
}

func (q *Quiz) askQuestion(word Word, current, total int) bool {
	fmt.Printf("\n%sQuestion %d/%d%s\n", ColorBold, current, total, ColorReset)

	// Show English translation if available
	if word.English != "" {
		fmt.Printf("(%s%s%s)\n", ColorCyan, word.English, ColorReset)
	}

	fmt.Printf("\nWhat is the article for %s%s%s?\n", ColorBold, word.Word, ColorReset)
	fmt.Printf("  %s1.%s die\n", ColorYellow, ColorReset)
	fmt.Printf("  %s2.%s der\n", ColorYellow, ColorReset)
	fmt.Printf("  %s3.%s das\n", ColorYellow, ColorReset)
	fmt.Printf("\nType 1-3 or 'der/die/das'. '?': hint, 's': skip, 'q': quit quiz\n")

	for {
		fmt.Print("Your answer: ")
		answer := strings.TrimSpace(strings.ToLower(q.getInput()))

		switch answer {
		case "q", "quit", "exit":
			fmt.Printf("%sExiting quiz early...%s\n", ColorYellow, ColorReset)
			return false
		case "?", "h", "hint":
			printHint(word)
			continue
		case "s", "skip":
			q.markWrong(word, "(skip)")
			return true
		}

		userArticle, ok := q.parseArticle(answer)
		if !ok {
			fmt.Printf("%sInvalid input. Try 1/2/3 or der/die/das ('?': hint).%s\n", ColorRed, ColorReset)
			continue
		}

		if userArticle == word.Article {
			q.sessionStats.correct++
			fmt.Printf("%s✓ Correct!%s", ColorGreen, ColorReset)
			if word.Plural != "" {
				fmt.Printf(" (Plural: %s)\n", word.Plural)
			} else {
				fmt.Println()
			}
		} else {
			q.markWrong(word, userArticle)
		}
		return true
	}
}

func printHint(w Word) {
	bits := []string{}
	if w.English != "" {
		bits = append(bits, "EN: "+w.English)
	}
	if w.Category != "" {
		bits = append(bits, "Category: "+w.Category)
	}
	if w.Difficulty != "" {
		bits = append(bits, "Difficulty: "+w.Difficulty)
	}
	if w.Plural != "" {
		bits = append(bits, "Plural: "+w.Plural)
	}
	if len(bits) == 0 {
		fmt.Println("No hint available.")
		return
	}
	fmt.Printf("Hint: %s\n", strings.Join(bits, " | "))
}

func (q *Quiz) markWrong(word Word, userArticle string) {
	fmt.Printf("%s✗ Wrong!%s The correct answer is %s%s%s %s\n",
		ColorRed, ColorReset, ColorGreen, word.Article, ColorReset, word.Word)

	q.sessionStats.mistakes = append(q.sessionStats.mistakes, MistakeInfo{
		word:          word,
		userAnswer:    userArticle,
		correctAnswer: word.Article,
	})

	// Track mistakes for practice mode
	q.stats.WordStats[word.Word]++
}

func (q *Quiz) parseArticle(in string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "1", "die", "f", "fem", "feminine":
		return "die", true
	case "2", "der", "r", "m", "masc", "masculine":
		return "der", true
	case "3", "das", "s", "n", "neut", "neuter":
		return "das", true
	}
	return "", false
}

func (q *Quiz) numberToArticle(num string) string {
	// Kept for backward compatibility; not used in new flow.
	num = strings.TrimSpace(num)
	switch num {
	case "1":
		return "die"
	case "2":
		return "der"
	case "3":
		return "das"
	default:
		return ""
	}
}

func (q *Quiz) showResults() {
	if q.sessionStats.total == 0 {
		fmt.Printf("\n%sNo answers recorded.%s\n", ColorYellow, ColorReset)
		return
	}

	duration := time.Since(q.sessionStats.startTime).Round(time.Second)
	percentage := float64(q.sessionStats.correct) / float64(q.sessionStats.total) * 100

	fmt.Printf("\n%s", strings.Repeat("=", 40))
	fmt.Printf("\n%sQuiz Complete!%s\n", ColorBold, ColorReset)
	fmt.Printf("Time: %v\n", duration)
	fmt.Printf("Score: %s%d/%d (%.1f%%)%s\n",
		getColorForScore(percentage), q.sessionStats.correct, q.sessionStats.total, percentage, ColorReset)

	// Update global stats
	q.stats.TotalQuizzes++
	q.stats.TotalQuestions += q.sessionStats.total
	q.stats.CorrectAnswers += q.sessionStats.correct

	// Show mistakes if any
	if len(q.sessionStats.mistakes) > 0 {
		fmt.Printf("\n%sMistakes to review:%s\n", ColorYellow, ColorReset)
		for _, m := range q.sessionStats.mistakes {
			fmt.Printf("• %s%s%s %s", ColorBold, m.correctAnswer, ColorReset, m.word.Word)
			if m.word.English != "" {
				fmt.Printf(" (%s)", m.word.English)
			}
			fmt.Printf(" - you said: %s%s%s\n", ColorRed, m.userAnswer, ColorReset)
		}
	}

	fmt.Println(strings.Repeat("=", 40))
}

func getColorForScore(percentage float64) string {
	switch {
	case percentage >= 90:
		return ColorGreen
	case percentage >= 70:
		return ColorYellow
	default:
		return ColorRed
	}
}

func (q *Quiz) ShowDetailedStats() {
	if q.stats.TotalQuestions == 0 {
		fmt.Printf("\n%sNo statistics available yet. Take a quiz first!%s\n", ColorYellow, ColorReset)
		return
	}

	accuracy := float64(q.stats.CorrectAnswers) / float64(q.stats.TotalQuestions) * 100

	fmt.Printf("\n%s%sOverall Statistics:%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("Total Quizzes: %d\n", q.stats.TotalQuizzes)
	fmt.Printf("Total Questions: %d\n", q.stats.TotalQuestions)
	fmt.Printf("Correct Answers: %d\n", q.stats.CorrectAnswers)
	fmt.Printf("Overall Accuracy: %s%.1f%%%s\n", getColorForScore(accuracy), accuracy, ColorReset)

	// Show most missed words
	if len(q.stats.WordStats) > 0 {
		// Collect and sort by error count
		type wordError struct {
			word   string
			errors int
		}
		sorted := make([]wordError, 0, len(q.stats.WordStats))
		for w, c := range q.stats.WordStats {
			if c > 0 {
				sorted = append(sorted, wordError{w, c})
			}
		}
		if len(sorted) == 0 {
			return
		}

		sort.Slice(sorted, func(i, j int) bool { return sorted[i].errors > sorted[j].errors })

		fmt.Printf("\n%sMost Challenging Words:%s\n", ColorYellow, ColorReset)
		top := 5
		if len(sorted) < top {
			top = len(sorted)
		}
		for i := 0; i < top; i++ {
			we := sorted[i]
			// Find the word to get its article
			for _, w := range q.words {
				if w.Word == we.word {
					fmt.Printf("• %s %s - missed %d time(s)\n", w.Article, w.Word, we.errors)
					break
				}
			}
		}
	}
}

func (q *Quiz) ShowPracticeMode() {
	// Collect unique challenging words
	challenging := make([]Word, 0, len(q.words))
	for _, word := range q.words {
		if q.stats.WordStats[word.Word] > 0 {
			challenging = append(challenging, word)
		}
	}

	if len(challenging) == 0 {
		fmt.Printf("\n%sNo mistakes to practice yet! Great job!%s\n", ColorGreen, ColorReset)
		return
	}

	// Weight by mistakes (cap repeats to 3)
	practiceWords := make([]Word, 0, len(challenging)*2)
	for _, word := range challenging {
		mistakes := q.stats.WordStats[word.Word]
		repeats := mistakes + 1
		if repeats > 3 {
			repeats = 3
		}
		for i := 0; i < repeats; i++ {
			practiceWords = append(practiceWords, word)
		}
	}

	fmt.Printf("\n%sPractice Mode: Focusing on %d challenging words%s\n",
		ColorYellow, len(challenging), ColorReset)

	numQuestions := 10
	if numQuestions > len(practiceWords) {
		numQuestions = len(practiceWords)
	}

	// Shuffle and start practice quiz
	q.rng.Shuffle(len(practiceWords), func(i, j int) {
		practiceWords[i], practiceWords[j] = practiceWords[j], practiceWords[i]
	})

	// Reset session stats
	q.sessionStats.correct = 0
	q.sessionStats.total = numQuestions
	q.sessionStats.mistakes = []MistakeInfo{}
	q.sessionStats.startTime = time.Now()

	answered := 0
	for i := 0; i < numQuestions; i++ {
		if cont := q.askQuestion(practiceWords[i], i+1, numQuestions); !cont {
			q.sessionStats.total = answered
			break
		}
		answered++
	}

	q.showResults()
}

func (q *Quiz) getInput() string {
	text, err := q.reader.ReadString('\n')
	if err != nil {
		// If we captured some text before error, return it; otherwise exit gracefully
		t := strings.TrimSpace(text)
		if t != "" {
			return t
		}
		fmt.Println()
		fmt.Printf("%sInput closed. Exiting...%s\n", ColorYellow, ColorReset)
		q.SaveStats()
		os.Exit(0)
	}
	return strings.TrimSpace(text)
}