package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ANSI color codes for better terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

type Word struct {
	Word       string   `json:"word"`
	Article    string   `json:"article"`
	English    string   `json:"english,omitempty"`
	Category   string   `json:"category,omitempty"`
	Difficulty string   `json:"difficulty,omitempty"`
	Plural     string   `json:"plural,omitempty"`
}

type Words struct {
	Version string `json:"version"`
	Data    []Word `json:"data"`
}

type Stats struct {
	TotalQuizzes   int            `json:"total_quizzes"`
	TotalQuestions int            `json:"total_questions"`
	CorrectAnswers int            `json:"correct_answers"`
	WordStats      map[string]int `json:"word_stats"` // tracks mistakes per word
}

type Quiz struct {
	words        []Word
	stats        *Stats
	reader       *bufio.Reader
	sessionStats struct {
		correct   int
		total     int
		mistakes  []MistakeInfo
		startTime time.Time
	}
}

type MistakeInfo struct {
	word        Word
	userAnswer  string
	correctAnswer string
}

func main() {
	quiz := NewQuiz()
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
	}
}

func (q *Quiz) LoadWords(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
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
		return // Stats file doesn't exist yet, that's OK
	}
	defer file.Close()

	json.NewDecoder(file).Decode(q.stats)
}

func (q *Quiz) SaveStats() {
	dataDir := getDataDir()
	os.MkdirAll(dataDir, 0755)
	
	statsFile := filepath.Join(dataDir, "stats.json")
	file, err := os.Create(statsFile)
	if err != nil {
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(q.stats)
}

func getDataDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".german_quiz")
}

func (q *Quiz) ShowWelcome() {
	fmt.Printf("%s%s=== German Article Quiz ===%s\n\n", ColorBold, ColorBlue, ColorReset)
	
	if q.stats.TotalQuizzes > 0 {
		accuracy := float64(q.stats.CorrectAnswers) / float64(q.stats.TotalQuestions) * 100
		fmt.Printf("Welcome back! Your overall accuracy: %s%.1f%%%s\n", ColorCyan, accuracy, ColorReset)
		fmt.Printf("Total quizzes completed: %d\n\n", q.stats.TotalQuizzes)
	}
}

func (q *Quiz) RunGameLoop() {
	for {
		q.ShowMenu()
		choice := q.getInput()

		switch choice {
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
	switch q.getInput() {
	case "2":
		difficulty = "easy"
	case "3":
		difficulty = "medium"
	case "4":
		difficulty = "hard"
	}

	q.StartQuiz(num, difficulty)
}

func (q *Quiz) StartQuiz(numQuestions int, difficulty string) {
	// Filter words based on difficulty if specified
	availableWords := q.words
	if difficulty != "" {
		filtered := []Word{}
		for _, w := range q.words {
			if w.Difficulty == difficulty || w.Difficulty == "" {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			availableWords = filtered
		}
	}

	if numQuestions > len(availableWords) {
		numQuestions = len(availableWords)
	}

	// Shuffle words
	shuffled := make([]Word, len(availableWords))
	copy(shuffled, availableWords)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Reset session stats
	q.sessionStats.correct = 0
	q.sessionStats.total = numQuestions
	q.sessionStats.mistakes = []MistakeInfo{}
	q.sessionStats.startTime = time.Now()

	fmt.Printf("\n%s%sStarting quiz with %d questions...%s\n", ColorBold, ColorCyan, numQuestions, ColorReset)
	fmt.Println(strings.Repeat("-", 40))

	// Run quiz
	for i := 0; i < numQuestions; i++ {
		q.askQuestion(shuffled[i], i+1, numQuestions)
	}

	q.showResults()
}

func (q *Quiz) askQuestion(word Word, current, total int) {
	fmt.Printf("\n%sQuestion %d/%d%s\n", ColorBold, current, total, ColorReset)
	
	// Show English translation if available
	if word.English != "" {
		fmt.Printf("(%s%s%s)\n", ColorCyan, word.English, ColorReset)
	}

	fmt.Printf("\nWhat is the article for %s%s%s?\n", ColorBold, word.Word, ColorReset)
	fmt.Printf("  %s1.%s die\n", ColorYellow, ColorReset)
	fmt.Printf("  %s2.%s der\n", ColorYellow, ColorReset)
	fmt.Printf("  %s3.%s das\n", ColorYellow, ColorReset)
	fmt.Print("\nYour answer (1-3): ")

	answer := q.getInput()
	userArticle := q.numberToArticle(answer)
	
	if userArticle == word.Article {
		q.sessionStats.correct++
		fmt.Printf("%s✓ Correct!%s", ColorGreen, ColorReset)
		if word.Plural != "" {
			fmt.Printf(" (Plural: %s)\n", word.Plural)
		} else {
			fmt.Println()
		}
	} else {
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
}

func (q *Quiz) numberToArticle(num string) string {
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
		fmt.Printf("\n%sMost Challenging Words:%s\n", ColorYellow, ColorReset)
		type wordError struct {
			word   string
			errors int
		}
		
		// Sort by error count
		var sortedErrors []wordError
		for word, count := range q.stats.WordStats {
			if count > 0 {
				sortedErrors = append(sortedErrors, wordError{word, count})
			}
		}
		
		// Simple sort
		for i := 0; i < len(sortedErrors); i++ {
			for j := i + 1; j < len(sortedErrors); j++ {
				if sortedErrors[j].errors > sortedErrors[i].errors {
					sortedErrors[i], sortedErrors[j] = sortedErrors[j], sortedErrors[i]
				}
			}
		}

		// Show top 5
		for i := 0; i < len(sortedErrors) && i < 5; i++ {
			// Find the word to get its article
			for _, w := range q.words {
				if w.Word == sortedErrors[i].word {
					fmt.Printf("• %s %s - missed %d time(s)\n", 
						w.Article, w.Word, sortedErrors[i].errors)
					break
				}
			}
		}
	}
}

func (q *Quiz) ShowPracticeMode() {
	// Create a list of words weighted by mistakes
	practiceWords := []Word{}
	
	for _, word := range q.words {
		mistakes := q.stats.WordStats[word.Word]
		// Add word multiple times based on mistakes
		repeats := mistakes + 1
		if mistakes == 0 {
			continue // Skip words never missed
		}
		for i := 0; i < repeats && i < 3; i++ {
			practiceWords = append(practiceWords, word)
		}
	}

	if len(practiceWords) == 0 {
		fmt.Printf("\n%sNo mistakes to practice yet! Great job!%s\n", ColorGreen, ColorReset)
		return
	}

	fmt.Printf("\n%sPractice Mode: Focusing on %d challenging words%s\n", 
		ColorYellow, len(q.stats.WordStats), ColorReset)
	
	numQuestions := 10
	if numQuestions > len(practiceWords) {
		numQuestions = len(practiceWords)
	}

	// Shuffle and start practice quiz
	rand.Shuffle(len(practiceWords), func(i, j int) {
		practiceWords[i], practiceWords[j] = practiceWords[j], practiceWords[i]
	})

	q.sessionStats.correct = 0
	q.sessionStats.total = numQuestions
	q.sessionStats.mistakes = []MistakeInfo{}
	q.sessionStats.startTime = time.Now()

	for i := 0; i < numQuestions; i++ {
		q.askQuestion(practiceWords[i], i+1, numQuestions)
	}

	q.showResults()
}

func (q *Quiz) getInput() string {
	text, _ := q.reader.ReadString('\n')
	return strings.TrimSpace(text)
}
